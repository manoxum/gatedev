// Configurações do painel guardadas no Postgres (tabela panel_config) -
// antes viviam no .env do host. A senha do nginx-ui nunca volta do
// backend: só o booleano dizendo se já existe uma configurada.
export interface PanelSettings {
  /** CN que será usado se/quando uma CA nova for gerada. */
  caCommonName: string;
  /** CN da CA que existe hoje (somente leitura). */
  caCurrentCommonName: string;
  caGenerated: boolean;
  nginxUiUsername: string;
  nginxUiConfigured: boolean;
}

// Campos omitidos não são alterados - por isso o PATCH é parcial e a senha
// só viaja quando o operador realmente digitou uma nova.
export interface PanelSettingsUpdate {
  caCommonName?: string;
  nginxUiUsername?: string;
  nginxUiPassword?: string;
}
