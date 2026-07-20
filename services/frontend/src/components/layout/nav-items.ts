import { LayoutDashboard, Wifi, Globe, ShieldCheck, Network, Settings, type LucideIcon } from "lucide-react";

export interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}

export const navItems: NavItem[] = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/hotspot", label: "Hotspot", icon: Wifi },
  { to: "/dns", label: "DNS", icon: Globe },
  { to: "/bindnets", label: "Bindnets", icon: Network },
  { to: "/certificates", label: "Certificados", icon: ShieldCheck },
  { to: "/settings", label: "Configurações", icon: Settings },
];
