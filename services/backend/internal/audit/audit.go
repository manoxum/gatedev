// Package audit escreve uma trilha minima de auditoria das acoes do painel
// (login/logout, mudancas de config, emissao/revogacao de certificado,
// start/stop de hotspot/dns) numa colecao Mongo dedicada - NUNCA persiste
// logs de containers (isso continua sendo so streaming ao vivo via worker,
// ver internal/workerapi / LogsPanel.tsx).
package audit

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"bindnet/backend/internal/platform/config"
)

type Client struct {
	collection *mongo.Collection
	client     *mongo.Client
}

type auditEvent struct {
	Type      string         `bson:"type"`
	Username  string         `bson:"username,omitempty"`
	Details   map[string]any `bson:"details,omitempty"`
	CreatedAt time.Time      `bson:"createdAt"`
}

func Open() (*Client, error) {
	uri := fmt.Sprintf(
		"mongodb://%s:%s@%s:%s",
		config.Getenv("MONGO_USER", "bindnet"),
		config.Getenv("MONGO_PASSWORD", ""),
		config.Getenv("MONGO_HOST", "mongo"),
		config.Getenv("MONGO_PORT", "27017"),
	)
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		return nil, err
	}
	db := client.Database(config.Getenv("MONGO_DB", "bindnet"))
	return &Client{collection: db.Collection("audit_log"), client: client}, nil
}

// MongoClient expoe o *mongo.Client subjacente - usado por
// internal/hotspot/store para abrir a colecao de trace de credito na mesma
// conexao (ver openCreditTrace).
func (a *Client) MongoClient() *mongo.Client {
	return a.client
}

// Record nunca propaga erro - falha ao gravar auditoria nao pode derrubar a
// acao principal do usuario, so loga o problema.
func (a *Client) Record(ctx context.Context, eventType, username string, details map[string]any) {
	_, err := a.collection.InsertOne(ctx, auditEvent{
		Type: eventType, Username: username, Details: details, CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("[backend] erro ao registrar auditoria (%s): %v", eventType, err)
	}
}

func (a *Client) Disconnect(ctx context.Context) {
	_ = a.client.Disconnect(ctx)
}

// Ping verifica se a conexao com o Mongo continua saudavel - usado pelo
// checklist de status do assistente de configuracao inicial.
func (a *Client) Ping(ctx context.Context) error {
	return a.client.Ping(ctx, nil)
}
