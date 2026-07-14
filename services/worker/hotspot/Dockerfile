FROM alpine:3.22

# grep: substitui o applet grep do BusyBox pelo GNU grep. O create_ap
# usa "grep -E '{.* managed.* AP.*}'" (regex com "{" solto) pra checar
# se o adaptador suporta AP+estacao simultaneos via "iw phy info" - o
# BusyBox grep trata "{" sem bound valido como erro de sintaxe e falha
# sempre, fazendo o create_ap concluir (errado, em qualquer adaptador)
# que o modo concorrente nao e suportado. GNU grep trata "{" solto como
# literal (extensao GNU), igual o create_ap espera.
# util-linux-misc: pacote separado do "util-linux" no Alpine que traz
# o binario "rfkill" (usado por ensure_wifi_radio_unblocked, ver
# regulatory.sh) - o Alpine nao tem um pacote "rfkill" avulso.
RUN apk add --no-cache \
    bash \
    curl \
    dnsmasq \
    grep \
    iproute2 \
    iptables \
    iw \
    kea-dhcp4 \
    postgresql-client \
    procps \
    util-linux \
    util-linux-misc \
    wireless-tools

# hostapd: fixado em 2.10-r6 (repo da Alpine 3.20, ultima versao antes
# do pulo pra 2.11 na Alpine 3.21) em vez do "hostapd" solto do repo da
# imagem base (Alpine 3.22 = hostapd 2.11), por causa de uma regressao
# conhecida do codigo inicial de IEEE 802.11be/MLO (Wi-Fi 7) da 2.11
# (relatada tambem por outros usuarios de create_ap/hostapd fora deste
# repo). ATENCAO: essa mesma assinatura de log ("Failed to set beacon
# parameters" + "nl80211: kernel reports: key not allowed" + interface
# disabled/enabled em loop, ver watchdog.sh) tambem reproduz com o
# hostapd 2.10 pinado aqui - confirmado cruzando o log do container com
# os logs do NetworkManager (journalctl -u NetworkManager) no mesmo
# timestamp: o gatilho real e o NetworkManager continuando "dono"
# (sys-iface-state: managed) da placa fisica em uso pelo hotspot
# (--no-virt) e tentando escanear/reassociar essa mesma placa as redes
# Wi-Fi salvas do usuario ENQUANTO o hostapd tenta manter o AP nela -
# a disputa pelo radio derruba o beacon. Causa raiz: unmanageWifiInterfaceIfIdle
# (services/worker/controller/compose.go) so desgerenciava a placa
# quando ela estava OCIOSA no momento da checagem - se o usuario tinha
# qualquer rede Wi-Fi (sem relacao nenhuma com o hotspot, ex.: internet
# via Ethernet) associada a essa placa nesse instante, a funcao
# preservava essa associacao "pra nao derrubar o Wi-Fi cliente do
# usuario", mesmo quando INTERNET_INTERFACE nao era essa mesma placa e
# portanto nao havia motivo nenhum pra preservar nada. Corrigido: agora
# so preserva a associacao quando WIFI_INTERFACE == INTERNET_INTERFACE
# de verdade (Wi-Fi para Wi-Fi, AP+STA concorrente na mesma placa) -
# qualquer outra combinacao sempre desgerencia a placa antes do hotspot
# subir, independente do que ela estiver associada no momento.
# O pin da 2.10 continua valendo a pena (evita a regressao de MLO em
# si), mas nao assuma que ele sozinho elimina essa cascata de beacon -
# watchdog.sh continua sendo a rede de seguranca de qualquer forma.
# libnl3 (dependencia do hostapd) continua vindo do repo normal da
# imagem base.
RUN apk add --no-cache hostapd=2.10-r6 \
    --repository=https://dl-cdn.alpinelinux.org/alpine/v3.20/main

COPY patch-create-ap.sh /tmp/patch-create-ap.sh
RUN curl -fsSL https://raw.githubusercontent.com/oblique/create_ap/master/create_ap \
	      -o /usr/local/bin/create_ap \
	    && sed -i 's/24\[0-9\]\[0-9\] MHz/24[0-9][0-9]\\(\\.0\\)\\? MHz/' /usr/local/bin/create_ap \
	    && sh /tmp/patch-create-ap.sh \
	    && rm /tmp/patch-create-ap.sh \
	    && chmod +x /usr/local/bin/create_ap

COPY entrypoint.sh channel.sh interfaces.sh regulatory.sh watchdog.sh history.sh /usr/local/bin/
RUN mv /usr/local/bin/entrypoint.sh /usr/local/bin/hotspot-entrypoint.sh \
    && chmod +x /usr/local/bin/hotspot-entrypoint.sh /usr/local/bin/channel.sh /usr/local/bin/interfaces.sh /usr/local/bin/regulatory.sh /usr/local/bin/watchdog.sh /usr/local/bin/history.sh

ENTRYPOINT ["/usr/local/bin/hotspot-entrypoint.sh"]
