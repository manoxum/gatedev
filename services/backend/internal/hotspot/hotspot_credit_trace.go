// hotspot_credit_trace.go grava e le o trace bruto de debito de credito
// (uma linha por ciclo de reconciliacao que desconta trafego do saldo)
// na colecao Mongo hotspot_credit_debits - alto volume (uma linha a
// cada ciclo do loop em hotspot_reconcile.go por dispositivo com
// credito habilitado), por isso mora no Mongo com TTL em vez do
// Postgres. Recarga manual/automatica e resgate de voucher continuam
// no Postgres (hotspot_credit_history.go) - sao eventos raros, ligados
// a acao humana ou dinheiro, sem motivo pra expirar sozinhos.
package hotspot

import (
	"bindnet/backend/internal/platform/config"
	"context"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type creditTraceClient struct {
	collection *mongo.Collection
}

type creditDebitEntry struct {
	MAC               string    `bson:"mac"`
	AmountBytes       int64     `bson:"amountBytes"`
	BalanceAfterBytes int64     `bson:"balanceAfterBytes"`
	CreatedAt         time.Time `bson:"createdAt"`
}

type creditDebitResponse struct {
	AmountBytes       int64  `json:"amountBytes"`
	BalanceAfterBytes int64  `json:"balanceAfterBytes"`
	CreatedAt         string `json:"createdAt"`
}

const creditTraceTTLIndexName = "createdAt_ttl"

// OpenCreditTrace reusa a conexao Mongo ja aberta para auditoria
// (mongoClient, ver audit.go) numa colecao separada - credito nao e
// trilha de auditoria do painel, e dado de negocio, mas nao vale abrir
// uma segunda conexao so por isso.
func OpenCreditTrace(ctx context.Context, mongoClient *mongo.Client) (*creditTraceClient, error) {
	collection := mongoClient.Database(config.Getenv("MONGO_DB", "bindnet")).Collection("hotspot_credit_debits")
	if err := ensureCreditTraceTTLIndex(ctx, collection); err != nil {
		return nil, err
	}
	return &creditTraceClient{collection: collection}, nil
}

func creditTraceRetentionSeconds() int32 {
	days, err := strconv.Atoi(config.Getenv("HOTSPOT_CREDIT_TRACE_RETENTION_DAYS", "180"))
	if err != nil || days <= 0 {
		days = 180
	}
	return int32(days * 24 * 60 * 60)
}

// ensureCreditTraceTTLIndex cria (ou ajusta via collMod) o indice TTL
// em createdAt a cada subida do backend - assim mudar
// HOTSPOT_CREDIT_TRACE_RETENTION_DAYS no .env passa a valer sem
// precisar recriar a colecao a mao (CreateOne nao aceita alterar as
// opcoes de um indice existente com o mesmo nome).
func ensureCreditTraceTTLIndex(ctx context.Context, collection *mongo.Collection) error {
	seconds := creditTraceRetentionSeconds()
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetName(creditTraceTTLIndexName).SetExpireAfterSeconds(seconds),
	})
	if err == nil {
		return nil
	}
	command := bson.D{
		{Key: "collMod", Value: collection.Name()},
		{Key: "index", Value: bson.D{
			{Key: "name", Value: creditTraceTTLIndexName},
			{Key: "expireAfterSeconds", Value: seconds},
		}},
	}
	return collection.Database().RunCommand(ctx, command).Err()
}

// recordDebit grava uma linha do trace de debito - chamada logo apos
// cada UPDATE de balance_bytes por consumo de trafego (ver
// debitDeviceCredit em hotspot_credit.go).
func (c *creditTraceClient) recordDebit(ctx context.Context, mac string, amountBytes, balanceAfterBytes int64) error {
	_, err := c.collection.InsertOne(ctx, creditDebitEntry{
		MAC: mac, AmountBytes: amountBytes, BalanceAfterBytes: balanceAfterBytes, CreatedAt: time.Now(),
	})
	return err
}

// listDebits devolve o trace de debito de um MAC, mais recente
// primeiro. since/until (opcionais) filtram por janela de tempo -
// usado pelo detalhe de consumo de uma sessao especifica
// (GET .../sessions/{id}/consumption); limit <= 0 nao limita - usado
// pelo extrato de credito completo (GET .../credit/history).
func (c *creditTraceClient) listDebits(ctx context.Context, mac string, since, until *time.Time, limit int64) ([]creditDebitResponse, error) {
	filter := bson.D{{Key: "mac", Value: mac}}
	if since != nil || until != nil {
		createdAtFilter := bson.D{}
		if since != nil {
			createdAtFilter = append(createdAtFilter, bson.E{Key: "$gte", Value: *since})
		}
		if until != nil {
			createdAtFilter = append(createdAtFilter, bson.E{Key: "$lte", Value: *until})
		}
		filter = append(filter, bson.E{Key: "createdAt", Value: createdAtFilter})
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	if limit > 0 {
		findOptions.SetLimit(limit)
	}
	cursor, err := c.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	entries := []creditDebitResponse{}
	for cursor.Next(ctx) {
		var entry creditDebitEntry
		if err := cursor.Decode(&entry); err != nil {
			return nil, err
		}
		entries = append(entries, creditDebitResponse{
			AmountBytes:       entry.AmountBytes,
			BalanceAfterBytes: entry.BalanceAfterBytes,
			CreatedAt:         entry.CreatedAt.Format(time.RFC3339),
		})
	}
	return entries, cursor.Err()
}
