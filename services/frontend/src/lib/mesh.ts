import { useQuery } from "@tanstack/react-query";
import { Globe2, Network, Route } from "lucide-react";
import { api } from "@/lib/api";

export interface DiscoveredPeer {
  address: string;
  nodeName: string;
  fingerprint?: string;
  source: string;
  lastSeenAt: string;
  domains?: string[];
}

export interface DiscoverRoute {
  domain: string;
  owner: string;
  ownerFingerprint?: string;
  nextHop: string;
  distance: number;
  source: string;
  state: string;
  lastSeenAt: string;
}

export interface DiscoveredServer {
  name: string;
  zone: string;
  source: string;
  kind: string;
  file?: string;
}

export interface LocalBindnetNode {
  nodeName: string;
  fingerprint?: string;
  domains: string[];
  port: string;
}

export type BindnetNodeKind = "local" | "direct" | "inferred";

export interface BindnetNode {
  id: string;
  name: string;
  address: string;
  host?: string;
  port?: string;
  fingerprint?: string;
  domains?: string[];
  kind: BindnetNodeKind;
  source: string;
  lastSeenAt?: string;
}

export interface MeshData {
  config: Record<string, string>;
  localNode: LocalBindnetNode;
  peers: DiscoveredPeer[];
  routes: DiscoverRoute[];
  localServices: DiscoveredServer[];
  nodes: BindnetNode[];
}

export function splitPeers(raw?: string) {
  return (raw ?? "")
    .split(/[,\s;]+/)
    .map((peer) => peer.trim())
    .filter(Boolean);
}

export function joinPeers(peers: string[]) {
  return peers.join(",");
}

export function normalizePeerAddress(value: string) {
  return value.trim().replace(/^https?:\/\//, "").replace(/\/+$/, "");
}

export async function unlinkPeerAddress(config: Record<string, string> | undefined, address: string) {
  const target = normalizePeerAddress(address);
  const peers = splitPeers(config?.DISCOVER_CONFIGURED_PEERS);
  const remaining = peers.filter((peer) => normalizePeerAddress(peer) !== target);

  if (!target || remaining.length === peers.length) {
    throw new Error("Este Bindnet não está vinculado manualmente.");
  }

  await api.patch("/dns/config", { DISCOVER_CONFIGURED_PEERS: joinPeers(remaining) });
  await api.post("/dns/apply");
}

export function remoteRouteMode(config?: Record<string, string>) {
  return config?.DISCOVER_REMOTE_ROUTES === "manual" ? "manual" : "auto";
}

export function peerHost(address: string) {
  return address.includes(":") ? address.split(":")[0] : address;
}

export function peerPort(address: string) {
  const parts = address.split(":");
  return parts.length > 1 ? parts[parts.length - 1] : "";
}

export function nodePath(id: string) {
  return `/bindnets/${encodeURIComponent(id)}`;
}

function unique(values: Array<string | undefined>) {
  return [...new Set(values.map((value) => value?.trim()).filter(Boolean) as string[])];
}

function isLocalRoute(route: DiscoverRoute, localName: string, localFingerprint?: string) {
  if (localFingerprint && route.ownerFingerprint === localFingerprint) return true;
  return route.owner === localName || route.source === "self";
}

export function remoteRoutes(data: MeshData) {
  const localName = data.localNode.nodeName || data.config.DISCOVER_NODE_NAME || "este-servidor";
  return data.routes.filter((route) => !isLocalRoute(route, localName, data.localNode.fingerprint));
}

export function buildNodes(
  config: Record<string, string>,
  peers: DiscoveredPeer[],
  routes: DiscoverRoute[],
  localServices: DiscoveredServer[],
  localNode: LocalBindnetNode,
): BindnetNode[] {
  const localName = localNode.nodeName?.trim() || config.DISCOVER_NODE_NAME?.trim() || "este-servidor";
  const nodes = new Map<string, BindnetNode>();
  const peerByAddress = new Map(peers.map((peer) => [normalizePeerAddress(peer.address), peer]));
  const directByHost = new Map<string, BindnetNode>();

  nodes.set("local", {
    id: "local",
    name: localName,
    address: `127.0.0.1:${localNode.port || config.DISCOVER_PORT || "8531"}`,
    host: "127.0.0.1",
    port: localNode.port || config.DISCOVER_PORT || "8531",
    fingerprint: localNode.fingerprint,
    domains: unique([...(localNode.domains ?? []), ...localServices.map((service) => service.name)]),
    kind: "local",
    source: "self",
  });

  for (const address of splitPeers(config.DISCOVER_CONFIGURED_PEERS)) {
    const normalized = normalizePeerAddress(address);
    const meta = peerByAddress.get(normalized);
    const node: BindnetNode = {
      id: `peer:${address}`,
      name: meta?.nodeName || address,
      address,
      host: peerHost(address),
      port: peerPort(address),
      fingerprint: meta?.fingerprint,
      domains: meta?.domains,
      kind: "direct",
      source: "manual",
      lastSeenAt: meta?.lastSeenAt,
    };
    nodes.set(node.id, node);
    directByHost.set(peerHost(address), node);
  }

  for (const route of routes) {
    if (isLocalRoute(route, localName, localNode.fingerprint)) continue;
    const direct = directByHost.get(peerHost(route.source));
    if (!direct || route.distance !== 1) continue;
    if (direct.name === direct.address && route.owner) {
      direct.name = route.owner;
    }
    if (!direct.fingerprint && route.ownerFingerprint) {
      direct.fingerprint = route.ownerFingerprint;
    }
    direct.domains = unique([...(direct.domains ?? []), route.domain]);
    direct.lastSeenAt = direct.lastSeenAt || route.lastSeenAt;
  }

  const directFingerprints = new Set(
    [...directByHost.values()].map((node) => node.fingerprint).filter(Boolean) as string[],
  );
  const directNames = new Set([...directByHost.values()].map((node) => node.name).filter(Boolean));

  for (const route of routes) {
    if (!route.owner || isLocalRoute(route, localName, localNode.fingerprint)) continue;
    if (route.ownerFingerprint && directFingerprints.has(route.ownerFingerprint)) continue;
    if (directNames.has(route.owner)) continue;
    const id = route.ownerFingerprint ? `owner:${route.ownerFingerprint}` : `owner:${route.owner}`;
    if (!nodes.has(id)) {
      nodes.set(id, {
        id,
        name: route.owner,
        address: route.nextHop ? `via ${route.nextHop}` : "rota aprendida",
        host: route.nextHop || peerHost(route.source),
        fingerprint: route.ownerFingerprint,
        domains: [route.domain],
        kind: "inferred",
        source: route.distance <= 1 ? "rota direta" : "rota indireta",
        lastSeenAt: route.lastSeenAt,
      });
    } else {
      const node = nodes.get(id);
      if (node) {
        node.domains = unique([...(node.domains ?? []), route.domain]);
      }
    }
  }

  return [...nodes.values()].sort((a, b) => {
    const order = { local: 0, direct: 1, inferred: 2 };
    return order[a.kind] - order[b.kind] || a.name.localeCompare(b.name);
  });
}

export function useMeshData() {
  return useQuery<MeshData>({
    queryKey: ["bindnets", "mesh"],
    queryFn: async () => {
      const [config, peers, routes, localServices, localNode] = await Promise.all([
        api.get<Record<string, string>>("/dns/config"),
        api.get<DiscoveredPeer[]>("/dns/peers"),
        api.get<DiscoverRoute[]>("/dns/routes"),
        api.get<DiscoveredServer[]>("/dns/discovered-servers"),
        api.get<LocalBindnetNode>("/dns/node"),
      ]);
      return { config, localNode, peers, routes, localServices, nodes: buildNodes(config, peers, routes, localServices, localNode) };
    },
    refetchInterval: 10000,
  });
}

export function nodeTone(kind: BindnetNodeKind) {
  if (kind === "local") return "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
  if (kind === "direct") return "border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300";
  return "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300";
}

export function nodeLabel(kind: BindnetNodeKind) {
  if (kind === "local") return "local";
  if (kind === "direct") return "direto";
  return "indireto";
}

export function formatSeen(value?: string) {
  if (!value) return "sem leitura";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function serviceRowsForNode(node: BindnetNode, data: MeshData) {
  if (node.kind === "local") {
    return data.localServices.map((service) => ({
      name: service.name,
      detail: service.kind,
      state: "local",
      via: "nginx-ui",
    }));
  }

  const routes = data.routes.filter((route) => {
    if (node.fingerprint && route.ownerFingerprint === node.fingerprint) return true;
    if (!node.fingerprint && route.owner === node.name) return true;
    return node.kind === "direct" && peerHost(route.source) === peerHost(node.address) && route.distance === 1;
  });

  return routes.map((route) => ({
    name: route.domain,
    detail: `${route.distance} salto${route.distance === 1 ? "" : "s"}`,
    state: route.state,
    via: route.nextHop || route.source,
  }));
}

export function routesViaNode(node: BindnetNode, routes: DiscoverRoute[]) {
  if (node.kind !== "direct") return [];
  return routes.filter((route) => {
    if (peerHost(route.source) !== peerHost(node.address)) return false;
    if (node.fingerprint && route.ownerFingerprint === node.fingerprint) return false;
    return route.owner !== node.name;
  });
}

export function neighborRows(node: BindnetNode, data: MeshData) {
  if (node.kind === "local") {
    return data.nodes.filter((candidate) => candidate.kind === "direct");
  }

  const indirectNames = new Set(routesViaNode(node, data.routes).map((route) => route.owner).filter(Boolean));
  return data.nodes.filter((candidate) => candidate.id === "local" || indirectNames.has(candidate.name));
}

export function metricCards(node: BindnetNode, data: MeshData) {
  const services = serviceRowsForNode(node, data).length;
  const neighbors = neighborRows(node, data).length;
  const routed = node.kind === "direct" ? routesViaNode(node, data.routes).length : remoteRoutes(data).length;
  return [
    { label: "Serviços", value: services, icon: Globe2 },
    { label: "Vizinhos", value: neighbors, icon: Network },
    { label: "Rotas", value: routed, icon: Route },
  ];
}
