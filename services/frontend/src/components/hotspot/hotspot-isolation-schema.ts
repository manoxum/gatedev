import { z } from "zod";
import type { HotspotCommRule, HotspotCommRuleRequest } from "@/components/hotspot/hotspot-isolation-types";

// "80", "80,443", "8000-8100", "53,80,1000-2000" - portas de destino.
const PORT_LIST_PATTERN = /^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$/;
// IPv4 simples ou CIDR (destino externo da zona wan).
const HOST_PATTERN = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/;

function isValidPortList(list: string): boolean {
  if (!PORT_LIST_PATTERN.test(list)) return false;
  return list.split(",").every((part) =>
    part.split("-").every((p) => {
      const n = Number(p);
      return Number.isInteger(n) && n >= 1 && n <= 65535;
    }),
  );
}

// O formulário é guiado primeiro pela ZONA:
//  - clients: entre clientes (modalidade within-profile ou endpoints);
//  - wan: cliente -> internet (origem + protocolo/portas + host externo);
//  - local: cliente -> painel/gateway (origem + protocolo/portas).
export const hotspotCommRuleFormSchema = z
  .object({
    zone: z.enum(["clients", "wan", "local"]),
    // clients
    scope: z.enum(["within-profile", "endpoints"]),
    profileRef: z.string(),
    sourceKind: z.enum(["profile", "device", "any"]),
    sourceRef: z.string(),
    targetKind: z.enum(["profile", "device", "any"]),
    targetRef: z.string(),
    directionUi: z.enum(["to", "from", "both"]),
    // wan
    dstHost: z.string(),
    // L4 (opcional)
    protocol: z.enum(["any", "tcp", "udp", "icmp"]),
    dstPorts: z.string(),
    // comuns
    action: z.enum(["allow", "deny"]),
    enabled: z.boolean(),
    note: z.string(),
  })
  .superRefine((values, ctx) => {
    if (values.dstPorts.trim()) {
      if (values.protocol !== "tcp" && values.protocol !== "udp") {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["dstPorts"], message: "Portas só com protocolo TCP ou UDP" });
      } else if (!isValidPortList(values.dstPorts.trim())) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["dstPorts"], message: "Portas inválidas (ex.: 80,443,8000-8100)" });
      }
    }
    if (values.zone === "wan" && values.dstHost.trim() && !HOST_PATTERN.test(values.dstHost.trim())) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["dstHost"], message: "Use um IP ou CIDR (ex.: 1.2.3.4 ou 10.0.0.0/24)" });
    }
    if (values.zone !== "clients") {
      if (values.sourceKind !== "any" && !values.sourceRef) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["sourceRef"], message: "Escolha a origem" });
      }
      return;
    }
    if (values.scope === "within-profile") {
      if (!values.profileRef) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["profileRef"], message: "Escolha o perfil" });
      }
      return;
    }
    if (!values.sourceRef) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["sourceRef"], message: "Escolha a origem" });
    }
    if (values.targetKind !== "any" && !values.targetRef) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["targetRef"], message: "Escolha o destino" });
    }
    if (
      values.sourceKind === "device" &&
      values.targetKind === "device" &&
      values.sourceRef &&
      values.sourceRef === values.targetRef
    ) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ["targetRef"], message: "Origem e destino não podem ser o mesmo dispositivo" });
    }
  });

export type HotspotCommRuleFormValues = z.infer<typeof hotspotCommRuleFormSchema>;

export const emptyCommRuleFormValues: HotspotCommRuleFormValues = {
  zone: "clients",
  scope: "within-profile",
  profileRef: "",
  sourceKind: "profile",
  sourceRef: "",
  targetKind: "profile",
  targetRef: "",
  directionUi: "both",
  dstHost: "",
  protocol: "any",
  dstPorts: "",
  action: "allow",
  enabled: true,
  note: "",
};

// Uma regra "dentro do perfil" é reconhecível por ter as duas pontas
// no mesmo perfil - é assim que ela volta a abrir na modalidade certa.
export function isWithinProfileRule(rule: HotspotCommRule): boolean {
  return (
    rule.zone === "clients" &&
    rule.sourceKind === "profile" &&
    rule.targetKind === "profile" &&
    rule.sourceRef === rule.targetRef
  );
}

function l4Base(rule: HotspotCommRule) {
  return {
    protocol: rule.protocol,
    dstPorts: rule.dstPorts ?? "",
    action: rule.action,
    enabled: rule.enabled,
    note: rule.note ?? "",
  };
}

export function commRuleToFormValues(rule: HotspotCommRule): HotspotCommRuleFormValues {
  if (rule.zone !== "clients") {
    return {
      ...emptyCommRuleFormValues,
      ...l4Base(rule),
      zone: rule.zone,
      sourceKind: rule.sourceKind,
      sourceRef: rule.sourceRef,
      dstHost: rule.dstHost ?? "",
    };
  }
  if (isWithinProfileRule(rule)) {
    return { ...emptyCommRuleFormValues, ...l4Base(rule), zone: "clients", scope: "within-profile", profileRef: rule.sourceRef };
  }
  return {
    ...emptyCommRuleFormValues,
    ...l4Base(rule),
    zone: "clients",
    scope: "endpoints",
    sourceKind: rule.sourceKind,
    sourceRef: rule.sourceRef,
    targetKind: rule.targetKind,
    targetRef: rule.targetRef ?? "",
    directionUi: rule.direction,
  };
}

export function formValuesToCommRule(values: HotspotCommRuleFormValues): HotspotCommRuleRequest {
  const common = {
    zone: values.zone,
    protocol: values.protocol,
    dstPorts: values.dstPorts.trim() ? values.dstPorts.trim() : null,
    dstHost: values.zone === "wan" && values.dstHost.trim() ? values.dstHost.trim() : null,
    action: values.action,
    enabled: values.enabled,
    note: values.note.trim() ? values.note.trim() : null,
  };

  // Zonas wan/local: destino implicito (internet/gateway), sempre no
  // sentido cliente -> destino.
  if (values.zone !== "clients") {
    return {
      ...common,
      sourceKind: values.sourceKind,
      sourceRef: values.sourceKind === "any" ? "" : values.sourceRef,
      targetKind: "any",
      targetRef: null,
      direction: "to",
    };
  }

  if (values.scope === "within-profile") {
    return { ...common, sourceKind: "profile", sourceRef: values.profileRef, targetKind: "profile", targetRef: values.profileRef, direction: "both" };
  }

  const direction = values.directionUi === "both" ? "both" : "to";
  if (values.targetKind === "any") {
    return { ...common, sourceKind: values.sourceKind, sourceRef: values.sourceRef, targetKind: "any", targetRef: null, direction };
  }

  // "from" (destino → origem) é gravado trocando as pontas (o backend só
  // conhece "to"/"both").
  const swapped = values.directionUi === "from";
  return {
    ...common,
    sourceKind: swapped ? values.targetKind : values.sourceKind,
    sourceRef: swapped ? values.targetRef : values.sourceRef,
    targetKind: swapped ? values.sourceKind : values.targetKind,
    targetRef: swapped ? values.sourceRef : values.targetRef,
    direction,
  };
}
