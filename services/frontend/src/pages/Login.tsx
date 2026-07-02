import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Navigate } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { useLogin, useSession } from "@/hooks/useAuth";
import { ApiError } from "@/lib/api";

const loginSchema = z.object({
  username: z.string().min(1, "Informe o usuário"),
  password: z.string().min(1, "Informe a senha"),
});
type LoginForm = z.infer<typeof loginSchema>;

export function LoginPage() {
  const { data: session } = useSession();
  const login = useLogin();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({ resolver: zodResolver(loginSchema) });

  if (session?.username) {
    return <Navigate to="/" replace />;
  }

  async function onSubmit(data: LoginForm) {
    try {
      await login.mutateAsync(data);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : "Falha ao entrar";
      toast.error(message || "Usuário ou senha inválidos");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>bindnet</CardTitle>
          <CardDescription>Entre para gerenciar o hotspot, DNS e certificados locais.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
            <div className="space-y-2">
              <Label htmlFor="username">Usuário</Label>
              <Input id="username" autoComplete="username" {...register("username")} />
              {errors.username && <p className="text-sm text-destructive">{errors.username.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Senha</Label>
              <Input id="password" type="password" autoComplete="current-password" {...register("password")} />
              {errors.password && <p className="text-sm text-destructive">{errors.password.message}</p>}
            </div>
            <Button type="submit" className="w-full" disabled={login.isPending}>
              {login.isPending ? "Entrando..." : "Entrar"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
