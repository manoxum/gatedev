// Tipos do isolamento de clientes (aba Isolamento) - espelham as rotas
// /api/hotspot/isolation do backend (hotspot_isolation.go). "device"
// referencia um MAC, "profile" um id de perfil, "any" é o curinga de
// destino (targetRef nulo). direction "to" = a origem pode INICIAR
// tráfego para o destino (respostas voltam sozinhas); "both" = os dois
// podem iniciar.
export type CommEndpointKind = "device" | "profile";
export type CommTargetKind = "device" | "profile" | "any";
export type CommDirection = "to" | "both";
export type CommAction = "allow" | "deny";
// Base do firewall por zonas. Na Camada 1 só a zona 'clients' está
// implementada de ponta a ponta; 'wan'/'local' entram nas próximas.
export type CommZone = "clients" | "wan" | "local";
export type CommProtocol = "any" | "tcp" | "udp" | "icmp";

export interface HotspotCommRule {
  id: string;
  zone: CommZone;
  sourceKind: CommEndpointKind;
  sourceRef: string;
  targetKind: CommTargetKind;
  targetRef: string | null;
  direction: CommDirection;
  protocol: CommProtocol;
  dstPorts: string | null;
  dstHost: string | null;
  action: CommAction;
  enabled: boolean;
  note: string | null;
  createdAt: string;
  updatedAt: string;
}

export type HotspotCommRuleRequest = Omit<HotspotCommRule, "id" | "createdAt" | "updatedAt">;

export interface HotspotIsolationState {
  enabled: boolean;
  restartRequired?: boolean;
}
