FROM alpine:3.22

# grep: substitui o applet grep do BusyBox pelo GNU grep. O create_ap
# usa "grep -E '{.* managed.* AP.*}'" (regex com "{" solto) pra checar
# se o adaptador suporta AP+estacao simultaneos via "iw phy info" - o
# BusyBox grep trata "{" sem bound valido como erro de sintaxe e falha
# sempre, fazendo o create_ap concluir (errado, em qualquer adaptador)
# que o modo concorrente nao e suportado. GNU grep trata "{" solto como
# literal (extensao GNU), igual o create_ap espera.
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
    wireless-tools

# hostapd: fixado em 2.10-r6 (repo da Alpine 3.20, ultima versao antes
# do pulo pra 2.11 na Alpine 3.21) em vez do "hostapd" solto do repo da
# imagem base (Alpine 3.22 = hostapd 2.11), por causa de uma regressao
# conhecida do codigo inicial de IEEE 802.11be/MLO (Wi-Fi 7) da 2.11
# (relatada tambem por outros usuarios de create_ap/hostapd fora deste
# repo). ATENCAO: essa mesma assinatura de log ("Failed to set beacon
# parameters" + "nl80211: kernel reports: key not allowed" + interface
# disabled/enabled em loop, ver watchdog.sh) tambem reproduz com o
# hostapd 2.10 pinado aqui - ja confirmado cruzando o log do container
# com "iwlwifi ...: RF_KILL bit toggled" no kernel e o audit
# "radio-control" do NetworkManager no mesmo timestamp: o gatilho real
# observado foi um rfkill de verdade na placa fisica em uso pelo
# hotspot (--no-virt), porque o NetworkManager continuava "dono" dela -
# ver unmanagePhysicalInterfaceIfIdle em
# services/backend/hotspot_network.go.
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

COPY entrypoint.sh channel.sh interfaces.sh regulatory.sh watchdog.sh /usr/local/bin/
RUN mv /usr/local/bin/entrypoint.sh /usr/local/bin/hotspot-entrypoint.sh \
    && chmod +x /usr/local/bin/hotspot-entrypoint.sh /usr/local/bin/channel.sh /usr/local/bin/interfaces.sh /usr/local/bin/regulatory.sh /usr/local/bin/watchdog.sh

ENTRYPOINT ["/usr/local/bin/hotspot-entrypoint.sh"]
