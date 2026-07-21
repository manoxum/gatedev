import { z } from "zod";
import type { HotspotCommRule, HotspotCommRuleRequest } from "@/components/hotspot/hotspot-isolation-types";

// "80", "80,443", "8000-8100", "53,80,1000-2000" - portas de destino.
const PORT_LIST_PATTERN = /^\d{1,5}(-\d{1,5})?(,\d{1,5}(-\d{1,5})?)*$/;

function isValidPortList(list: string): boolean {
  if (!PORT_LIST_PATTERN.test(list)) return false;
  return list.split(",").every((part) =>
    part.split("-").every((p) => {
      const n = Number(p);
      return Number.isInteger(n) && n >= 1 && n <= 65535;
    }),
  );
}

// O formulário tem duas modalidades ("escopo"):
//  - within-profile: comunicação entre clientes do MESMO perfil. É
//    gravada como uma regra normal com origem e destino iguais ao
//    mesmo perfil (o motor de política trata isso como "interno").
//  - endpoints: comunicação entre uma origem e um destino distintos
//    (perfil ou cliente; o destino também pode ser "todos os clientes").
export const hotspotCommRuleFormSchema = z
  .object({
    scope: z.enum(["within-profile", "endpoints"]),
    // within-profile
    profileRef: z.string(),
    // endpoints
    sourceKind: z.enum(["profile", "device"]),
    sourceRef: z.string(),
    targetKind: z.enum(["profile", "device", "any"]),
    targetRef: z.string(),
    directionUi: z.enum(["to", "from", "both"]),
    // L4 (opcional): restringe a regra a um protocolo/portas de destino.
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
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["dstPorts"],
          message: "Portas só com protocolo TCP ou UDP",
        });
      } else if (!isValidPortList(values.dstPorts.trim())) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["dstPorts"],
          message: "Portas inválidas (ex.: 80,443,8000-8100)",
        });
      }
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
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["targetRef"],
        message: "Origem e destino não podem ser o mesmo dispositivo",
      });
    }
  });

export type HotspotCommRuleFormValues = z.infer<typeof hotspotCommRuleFormSchema>;

export const emptyCommRuleFormValues: HotspotCommRuleFormValues = {
  scope: "within-profile",
  profileRef: "",
  sourceKind: "profile",
  sourceRef: "",
  targetKind: "profile",
  targetRef: "",
  directionUi: "both",
  protocol: "any",
  dstPorts: "",
  action: "allow",
  enabled: true,
  note: "",
};

// Uma regra "dentro do perfil" é reconhecível por ter as duas pontas
// no mesmo perfil - é assim que ela volta a abrir na modalidade certa.
export function isWithinProfileRule(rule: HotspotCommRule): boolean {
  return rule.sourceKind === "profile" && rule.targetKind === "profile" && rule.sourceRef === rule.targetRef;
}

export function commRuleToFormValues(rule: HotspotCommRule): HotspotCommRuleFormValues {
  const base = {
    protocol: rule.protocol,
    dstPorts: rule.dstPorts ?? "",
    action: rule.action,
    enabled: rule.enabled,
    note: rule.note ?? "",
  };
  if (isWithinProfileRule(rule)) {
    return {
      ...emptyCommRuleFormValues,
      ...base,
      scope: "within-profile",
      profileRef: rule.sourceRef,
    };
  }
  return {
    ...emptyCommRuleFormValues,
    ...base,
    scope: "endpoints",
    sourceKind: rule.sourceKind,
    sourceRef: rule.sourceRef,
    targetKind: rule.targetKind,
    targetRef: rule.targetRef ?? "",
    directionUi: rule.direction,
  };
}

export function formValuesToCommRule(values: HotspotCommRuleFormValues): HotspotCommRuleRequest {
  // Campos comuns a todas as regras da Camada 1 (zona clients + L4). O
  // backend zera dst_ports quando o protocolo não é tcp/udp.
  const common = {
    zone: "clients" as const,
    protocol: values.protocol,
    dstPorts: values.dstPorts.trim() ? values.dstPorts.trim() : null,
    dstHost: null,
    action: values.action,
    enabled: values.enabled,
    note: values.note.trim() ? values.note.trim() : null,
  };

  if (values.scope === "within-profile") {
    // Origem e destino no mesmo perfil, sempre bidirecional - controla a
    // comunicação entre os clientes daquele perfil.
    return {
      ...common,
      sourceKind: "profile",
      sourceRef: values.profileRef,
      targetKind: "profile",
      targetRef: values.profileRef,
      direction: "both",
    };
  }

  const direction = values.directionUi === "both" ? "both" : "to";
  if (values.targetKind === "any") {
    return {
      ...common,
      sourceKind: values.sourceKind,
      sourceRef: values.sourceRef,
      targetKind: "any",
      targetRef: null,
      direction,
    };
  }

  // "from" (destino → origem) é gravado trocando as pontas, já que o
  // backend só conhece "to"/"both".
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
