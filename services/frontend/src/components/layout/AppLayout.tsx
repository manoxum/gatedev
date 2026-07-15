import { useCallback, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { LogOut, Menu, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sidebar } from "@/components/layout/Sidebar";
import { MobileNav } from "@/components/layout/MobileNav";
import { navItems } from "@/components/layout/nav-items";
import { useLogout, useSession } from "@/hooks/useAuth";
import { useTheme } from "@/hooks/useTheme";
import type { PageHeaderData, SetPageHeader } from "@/hooks/usePageHeader";

function activeNavItem(pathname: string) {
  return navItems.find((item) => (item.end ? pathname === item.to : pathname.startsWith(item.to)));
}

// Shell de altura fixa: só o <main> rola, header e sidebar ficam sempre visíveis.
export function AppLayout() {
  const { data: session } = useSession();
  const logout = useLogout();
  const { theme, toggleTheme } = useTheme();
  const location = useLocation();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [pageHeader, setPageHeader] = useState<PageHeaderData>({ title: "" });

  const setHeader = useCallback<SetPageHeader>((header) => {
    setPageHeader((current) => {
      if (current.title === header.title && current.description === header.description) {
        return current;
      }

      return header;
    });
  }, []);
  const nav = activeNavItem(location.pathname);
  const Icon = nav?.icon;
  const title = pageHeader.title || nav?.label || "";

  return (
    <div className="flex h-screen overflow-hidden bg-muted/30">
      <Sidebar />

      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-16 shrink-0 items-center justify-between border-b bg-background px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <Button
              variant="ghost"
              size="icon"
              className="sm:hidden"
              aria-label="Abrir menu"
              onClick={() => setMobileNavOpen(true)}
            >
              <Menu className="h-5 w-5" />
            </Button>
            {Icon && (
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                <Icon className="h-5 w-5" />
              </div>
            )}
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold leading-tight">{title}</h1>
              {pageHeader.description && (
                <p className="truncate text-xs text-muted-foreground leading-tight">{pageHeader.description}</p>
              )}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            {session?.username && <span className="hidden text-sm sm:inline">{session.username}</span>}
            <Button
              variant="ghost"
              size="sm"
              onClick={toggleTheme}
              aria-label={theme === "dark" ? "Ativar tema claro" : "Ativar tema escuro"}
            >
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => logout.mutate()}>
              <LogOut className="mr-2 h-4 w-4" />
              Sair
            </Button>
          </div>
        </header>
        <main className="scroll-area flex-1 overflow-y-auto p-4 sm:p-6">
          <div className="page-enter">
            <Outlet context={setHeader} />
          </div>
        </main>
      </div>

      <MobileNav open={mobileNavOpen} onOpenChange={setMobileNavOpen} />
    </div>
  );
}
