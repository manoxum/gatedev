import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ShieldCheck, ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { CertificateList } from "@/components/certificates/CertificateList";
import type { Certificate } from "@/components/certificates/certificate-types";
import { api, ApiError } from "@/lib/api";
import { usePageHeader } from "@/hooks/usePageHeader";

const issueSchema = z.object({
  domain: z.string().min(1, "Informe um domínio ou IP"),
});
type IssueForm = z.infer<typeof issueSchema>;

export function CertificatesPage() {
  usePageHeader({
    title: "Certificados (CA local)",
    description: "Emita, liste, revogue e baixe certificados assinados pela CA local do painel.",
  });

  const queryClient = useQueryClient();
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const certificates = useQuery<Certificate[]>({
    queryKey: ["certificates"],
    queryFn: () => api.get<Certificate[]>("/certificates"),
  });

  const revokedCertificates = useQuery<Certificate[]>({
    queryKey: ["certificates", "revoked"],
    queryFn: () => api.get<Certificate[]>("/certificates/revoked"),
  });

  const { register, handleSubmit, reset, formState: { errors } } = useForm<IssueForm>({
    resolver: zodResolver(issueSchema),
  });

  const issue = useMutation({
    mutationFn: (data: IssueForm) => api.post("/certificates", data),
    onSuccess: () => {
      toast.success("Certificado emitido.");
      reset();
      queryClient.invalidateQueries({ queryKey: ["certificates"] });
      queryClient.invalidateQueries({ queryKey: ["certificates", "revoked"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao emitir certificado"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/certificates/${id}`),
    onSuccess: () => {
      toast.success("Certificado revogado.");
      queryClient.invalidateQueries({ queryKey: ["certificates"] });
      queryClient.invalidateQueries({ queryKey: ["certificates", "revoked"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao revogar"),
  });

  const permanentDelete = useMutation({
    mutationFn: (id: string) => api.del(`/certificates/${id}/permanent`),
    onSuccess: () => {
      toast.success("Certificado eliminado.");
      queryClient.invalidateQueries({ queryKey: ["certificates"] });
      queryClient.invalidateQueries({ queryKey: ["certificates", "revoked"] });
      setConfirmDeleteId(null);
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao eliminar"),
  });

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">
        Nada escuta mais nas portas 80/443 - a emissão agora é sempre uma ação explícita aqui. O
        download/instalação da CA raiz agora fica em "Servidores Bindnet", junto com os outros nós da malha.
      </p>

      <Card>
        <CardHeader>
          <CardTitle>Emitir certificado</CardTitle>
          <CardDescription>Emite um novo certificado assinado pela CA local para o domínio/IP informado.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex items-end gap-2" onSubmit={handleSubmit((data) => issue.mutate(data))}>
            <div className="flex-1 space-y-2">
              <Label htmlFor="domain">Domínio ou IP</Label>
              <Input id="domain" placeholder="ex.: painel.local" {...register("domain")} />
              {errors.domain && <p className="text-sm text-destructive">{errors.domain.message}</p>}
            </div>
            <Button type="submit" disabled={issue.isPending}>
              Emitir
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Certificados</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="issued">
            <TabsList>
              <TabsTrigger value="issued">
                <ShieldCheck className="h-4 w-4" />
                Emitidos ({certificates.data?.length ?? 0})
              </TabsTrigger>
              <TabsTrigger value="revoked">
                <ShieldX className="h-4 w-4" />
                Revogados ({revokedCertificates.data?.length ?? 0})
              </TabsTrigger>
            </TabsList>
            <TabsContent value="issued">
              <CertificateList
                certificates={certificates.data ?? []}
                isLoading={certificates.isLoading}
                emptyMessage="Nenhum certificado emitido ainda."
                revokePending={revoke.isPending}
                onRevoke={(id) => revoke.mutate(id)}
              />
            </TabsContent>
            <TabsContent value="revoked">
              <CertificateList
                certificates={revokedCertificates.data ?? []}
                isLoading={revokedCertificates.isLoading}
                emptyMessage="Nenhum certificado revogado."
                revoked
                permanentDeletePending={permanentDelete.isPending}
                onPermanentDelete={(id) => setConfirmDeleteId(id)}
              />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={!!confirmDeleteId}
        onOpenChange={(open) => !open && setConfirmDeleteId(null)}
        title="Eliminar certificado definitivamente"
        description="Esta ação não pode ser desfeita. O certificado revogado será removido permanentemente da CA local."
        confirmLabel="Eliminar"
        variant="destructive"
        pending={permanentDelete.isPending}
        onConfirm={() => confirmDeleteId && permanentDelete.mutate(confirmDeleteId)}
      />
    </div>
  );
}
