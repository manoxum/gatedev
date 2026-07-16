package main

// rateUnit e a unidade de uma taxa configurada pelo admin: bits/s
// (Kb/Mb/Gb na UI, "kbit"/"mbit"/"gbit" no valor - mesmo sufixo que tc
// usa) ou bytes/s (KB/MB/GB na UI, "kbyte"/"mbyte"/"gbyte" no valor -
// o worker traduz para os sufixos tc kbps/mbps/gbps). Ver rate() em
// services/worker/controller/shaping_tc.go. Compartilhado entre limite
// de dispositivo (hotspot_device_limits.go) e perfil (hotspot_profiles.go).
type rateUnit = string

const (
	rateUnitKbit  rateUnit = "kbit"
	rateUnitMbit  rateUnit = "mbit"
	rateUnitGbit  rateUnit = "gbit"
	rateUnitKbyte rateUnit = "kbyte"
	rateUnitMbyte rateUnit = "mbyte"
	rateUnitGbyte rateUnit = "gbyte"
)
