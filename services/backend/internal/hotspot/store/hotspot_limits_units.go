package store

// RateUnit e a unidade de uma taxa configurada pelo admin: bits/s
// (Kb/Mb/Gb na UI, "kbit"/"mbit"/"gbit" no valor - mesmo sufixo que tc
// usa) ou bytes/s (KB/MB/GB na UI, "kbyte"/"mbyte"/"gbyte" no valor -
// o worker traduz para os sufixos tc kbps/mbps/gbps). Ver rate() em
// services/worker/controller/shaping_tc.go. Compartilhado entre limite
// de dispositivo (hotspot_device_limits.go) e perfil (hotspot_profiles.go).
type RateUnit = string

const (
	rateUnitKbit  RateUnit = "kbit"
	rateUnitMbit  RateUnit = "mbit"
	rateUnitGbit  RateUnit = "gbit"
	rateUnitKbyte RateUnit = "kbyte"
	rateUnitMbyte RateUnit = "mbyte"
	rateUnitGbyte RateUnit = "gbyte"
)
