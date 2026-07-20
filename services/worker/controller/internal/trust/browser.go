// browser_trust.go importa a CA local nas stores NSS proprias do
// Chrome/Chromium (~/.pki/nssdb) e do Firefox (~/.mozilla/firefox/*),
// que ignoram /usr/local/share/ca-certificates (ver handleInstallLocalCA
// em ca.go, que so cobre a store do sistema). So atua sobre
// /hosthome, montado a partir de /home do host inteiro
// (docker-compose.services.yml) - cada subdiretorio de /hosthome e
// tratado como o $HOME de um usuario do sistema. Se o volume nao
// existir, e um no-op silencioso em vez de erro, para nao quebrar
// instalacoes que ainda nao configuraram esse mount.
package trust

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	hostHomeRoot    = "/hosthome"
	caNickname      = "Bindnet Local CA"
	chromeNSSSubdir = ".pki/nssdb"
	firefoxSubdir   = ".mozilla/firefox"
)

type browserTrustResult struct {
	Store     string `json:"store"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Error     string `json:"error,omitempty"`
}

// importCAIntoBrowserStores tenta instalar a CA em cada NSS db
// encontrada sob /hosthome/<usuario>. Erros individuais nao
// interrompem as outras tentativas - cada store reporta seu proprio
// resultado.
func importCAIntoBrowserStores(certificatePEM string) []browserTrustResult {
	userDirs, err := os.ReadDir(hostHomeRoot)
	if err != nil {
		return nil
	}

	certFile, cleanup, err := writeTempCert(certificatePEM)
	if err != nil {
		return []browserTrustResult{{Store: "nss", Error: "falha ao preparar certificado temporario: " + err.Error()}}
	}
	defer cleanup()

	var results []browserTrustResult
	for _, entry := range userDirs {
		if !entry.IsDir() {
			continue
		}
		userHome := filepath.Join(hostHomeRoot, entry.Name())
		homeInfo, err := os.Stat(userHome)
		if err != nil {
			continue
		}
		uid, gid, ok := ownerOf(homeInfo)
		if !ok {
			continue
		}
		results = append(results, importCAForUser(userHome, certFile, uid, gid)...)
	}
	return results
}

func importCAForUser(userHome, certFile string, uid, gid int) []browserTrustResult {
	var results []browserTrustResult

	chromeDir := filepath.Join(userHome, chromeNSSSubdir)
	if err := os.MkdirAll(chromeDir, 0700); err == nil {
		chownRecursive(chromeDir, uid, gid)
		results = append(results, importIntoNSSDb("Chrome/Chromium", chromeDir, certFile, uid, gid))
	}

	firefoxRoot := filepath.Join(userHome, firefoxSubdir)
	profiles, _ := filepath.Glob(filepath.Join(firefoxRoot, "*.default*"))
	for _, profile := range profiles {
		info, err := os.Stat(profile)
		if err != nil || !info.IsDir() {
			continue
		}
		results = append(results, importIntoNSSDb("Firefox", profile, certFile, uid, gid))
	}

	return results
}

func importIntoNSSDb(store, dir, certFile string, uid, gid int) browserTrustResult {
	dbArg := "sql:" + dir

	if _, err := os.Stat(filepath.Join(dir, "cert9.db")); err != nil {
		if out, err := exec.Command("certutil", "-N", "-d", dbArg, "--empty-password").CombinedOutput(); err != nil {
			return browserTrustResult{Store: store, Path: dir, Error: strings.TrimSpace(string(out))}
		}
	}

	// Remove uma instalacao anterior da mesma CA antes de reinstalar -
	// certutil -A duplicaria o apelido em vez de substituir.
	_ = exec.Command("certutil", "-D", "-d", dbArg, "-n", caNickname).Run()

	out, err := exec.Command("certutil", "-A", "-d", dbArg, "-t", "C,,", "-n", caNickname, "-i", certFile).CombinedOutput()
	chownRecursive(dir, uid, gid)
	if err != nil {
		return browserTrustResult{Store: store, Path: dir, Error: strings.TrimSpace(string(out))}
	}
	return browserTrustResult{Store: store, Path: dir, Installed: true}
}

func writeTempCert(certificatePEM string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "bindnet-ca-*.crt")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(certificatePEM); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// ownerOf preserva o dono real do $HOME do host - o worker roda como
// root no container, e arquivos NSS deixados root-owned impedem o
// navegador (rodando como usuario normal) de escrever neles depois.
func ownerOf(info os.FileInfo) (uid, gid int, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}

func chownRecursive(root string, uid, gid int) {
	_ = filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_ = os.Chown(path, uid, gid)
		return nil
	})
}
