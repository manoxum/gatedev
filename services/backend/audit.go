// audit.go escreve uma trilha minima de auditoria das acoes do
// painel (login/logout, mudancas de config, emissao/revogacao de
// certificado, start/stop de hotspot/dns) numa colecao Mongo dedicada -
// NUNCA persiste logs de containers (isso continua sendo so streaming
// ao vivo via worker, ver workerclient.go/LogsPanel.tsx).
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type auditClient struct {
	collection *mongo.Collection
	client     *mongo.Client
}

type auditEvent struct {
	Type      string         `bson:"type"`
	Username  string         `bson:"username,omitempty"`
	Details   map[string]any `bson:"details,omitempty"`
	CreatedAt time.Time      `bson:"createdAt"`
}

func openMongo() (*auditClient, error) {
	uri := fmt.Sprintf(
		"mongodb://%s:%s@%s:%s",
		getenv("MONGO_USER", "bindnet"),
		getenv("MONGO_PASSWORD", ""),
		getenv("MONGO_HOST", "mongo"),
		getenv("MONGO_PORT", "27017"),
	)
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		return nil, err
	}
	db := client.Database(getenv("MONGO_DB", "bindnet"))
	return &auditClient{collection: db.Collection("audit_log"), client: client}, nil
}

// record nunca propaga erro - falha ao gravar auditoria nao pode
// derrubar a acao principal do usuario, so loga o problema.
func (a *auditClient) record(ctx context.Context, eventType, username string, details map[string]any) {
	_, err := a.collection.InsertOne(ctx, auditEvent{
		Type: eventType, Username: username, Details: details, CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("[backend] erro ao registrar auditoria (%s): %v", eventType, err)
	}
}

func (a *auditClient) disconnect(ctx context.Context) {
	_ = a.client.Disconnect(ctx)
}
