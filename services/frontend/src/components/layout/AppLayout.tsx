import { NavLink, Outlet } from "react-router-dom";
import { LayoutDashboard, Wifi, Globe, ShieldCheck, LogOut } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useLogout, useSession } from "@/hooks/useAuth";

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/hotspot", label: "Hotspot", icon: Wifi },
  { to: "/dns", label: "DNS", icon: Globe },
  { to: "/certificates", label: "Certificados", icon: ShieldCheck },
];

export function AppLayout() {
  const { data: session } = useSession();
  const logout = useLogout();

  return (
    <div className="flex min-h-screen bg-muted/30">
      <aside className="hidden w-60 flex-col border-r bg-background p-4 sm:flex">
        <div className="mb-6 px-2 text-lg font-semibold">bindnet</div>
        <nav className="flex flex-col gap-1">
          {navItems.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 rounded-md px-2 py-2 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                  isActive && "bg-accent text-accent-foreground",
                )
              }
            >
              <Icon className="h-4 w-4" />
              {label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="flex flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b bg-background px-6">
          <span className="text-sm text-muted-foreground">Painel de gestão da rede</span>
          <div className="flex items-center gap-3">
            {session?.username && <span className="text-sm">{session.username}</span>}
            <Button variant="ghost" size="sm" onClick={() => logout.mutate()}>
              <LogOut className="mr-2 h-4 w-4" />
              Sair
            </Button>
          </div>
        </header>
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
