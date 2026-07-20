package workerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client fala com o services/worker atraves do socket Unix
// compartilhado (worker_ipc) - o backend nunca acessa docker.sock,
// NetworkManager ou network_mode: host diretamente.
type Client struct {
	http *http.Client
}

func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 15 * time.Second}}
}

// call faz uma requisicao para a API interna do worker e decodifica a
// resposta JSON em dest, se dest != nil.
func (c *Client) Call(ctx context.Context, method, path string, body, dest any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://worker"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("worker retornou %d: %s", resp.StatusCode, message)
	}
	if dest != nil {
		return json.NewDecoder(resp.Body).Decode(dest)
	}
	return nil
}

// callText busca uma resposta em texto puro (nao JSON) - usado para
// buscar um trecho de logs para analise (ex.: extrair canal/banda
// resolvidos automaticamente pelo hotspot).
func (c *Client) CallText(ctx context.Context, path string, dest *strings.Builder) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://worker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(dest, resp.Body)
	return err
}

// streamLogs repassa ao ResponseWriter o stream de logs que o worker
// devolve, em tempo real (usado pelos paineis de log do frontend).
// since, quando nao vazio, e um timestamp RFC3339: o worker repassa
// como "--since" para o "docker compose logs", devolvendo so logs a
// partir dali (usado pelo "Limpar logs" do hotspot, ver
// hotspot_logs.go).
func (c *Client) StreamLogs(ctx context.Context, w http.ResponseWriter, container string, follow bool, since string) error {
	endpoint := fmt.Sprintf("http://worker/containers/%s/logs?tail=200", container)
	if follow {
		endpoint += "&follow=true"
	}
	if since != "" {
		endpoint += "&since=" + url.QueryEscape(since)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, werr := w.Write(buffer[:n]); werr != nil {
				return werr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
