// Opcoes de janela do grafico de velocidade (DeviceSpeedChart.tsx) -
// arquivo proprio, sem nenhum import pesado (react-apexcharts), pra
// HotspotDeviceDetail.tsx poder ler SPEED_CHART_DEFAULT_WINDOW_MINUTES
// sem desfazer o code-splitting: um import direto de dentro de
// DeviceSpeedChart.tsx arrastaria o modulo inteiro (e o ApexCharts
// junto) pro bundle principal, ja que import estatico nao faz
// tree-shaking por export nesse sentido.
export const SPEED_CHART_WINDOWS = [
  { minutes: 1, label: "1 min" },
  { minutes: 5, label: "5 min" },
  { minutes: 10, label: "10 min" },
  { minutes: 15, label: "15 min" },
  { minutes: 30, label: "30 min" },
  { minutes: 60, label: "1 hora" },
  { minutes: 360, label: "6 horas" },
  { minutes: 720, label: "12 horas" },
  { minutes: 1440, label: "1 dia" },
];

export const SPEED_CHART_DEFAULT_WINDOW_MINUTES = 15;
