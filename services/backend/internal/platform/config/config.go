// Package config le variaveis de ambiente com valor padrao - o unico
// ponto do backend que toca os.Getenv, para os demais pacotes dependerem
// so desta funcao.
package config

import "os"

func Getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
