// Package cache mantem o Redis usado como cache de leitura para hostname
// -> offset de loopback. O Postgres continua sendo a fonte de verdade (ver
// pacote store) - perder o Redis so custa uma rehidratacao.
package cache

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"

	"bindnet/dns-provider/internal/config"
	"bindnet/dns-provider/internal/store"
)

const redisHashKey = "local_dns_records"

func Open(ctx context.Context) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Getenv("REDIS_HOST", "redis"), config.Getenv("REDIS_PORT", "6379")),
		Password: config.Getenv("REDIS_PASSWORD", ""),
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return client, nil
}

// Hydrate carrega todos os registros ja persistidos no Postgres para
// dentro do Redis, na inicializacao do servico - assim o cache comeca
// quente, sem esperar cada hostname ser consultado de novo para reaparecer
// no Redis apos um restart.
func Hydrate(ctx context.Context, db *sql.DB, cache *redis.Client) error {
	records, err := store.LoadAllRecords(ctx, db)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	fields := make(map[string]any, len(records))
	for hostname, offset := range records {
		fields[hostname] = offset
	}
	if err := cache.HSet(ctx, redisHashKey, fields).Err(); err != nil {
		return err
	}
	log.Printf("[dns-provider] cache Redis hidratado com %d registro(s) do Postgres", len(records))
	return nil
}

// Offset busca o offset no Redis primeiro (rapido); em caso de cache miss,
// o chamador deve consultar/alocar no Postgres e gravar de volta no cache
// via StoreOffset.
func Offset(ctx context.Context, cache *redis.Client, hostname string) (int64, bool) {
	value, err := cache.HGet(ctx, redisHashKey, hostname).Result()
	if err != nil {
		return 0, false
	}
	offset, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return offset, true
}

func StoreOffset(ctx context.Context, cache *redis.Client, hostname string, offset int64) {
	if err := cache.HSet(ctx, redisHashKey, hostname, offset).Err(); err != nil {
		log.Printf("[dns-provider] aviso: falha ao gravar %s no cache Redis: %v", hostname, err)
	}
}
