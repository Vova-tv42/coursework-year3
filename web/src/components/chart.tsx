import ReactEChartsCore from "echarts-for-react/esm/core";
import { BarChart, LineChart } from "echarts/charts";
import {
  DataZoomComponent,
  GridComponent,
  TooltipComponent,
} from "echarts/components";
import * as echarts from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";

import { useTheme } from "@/components/theme-provider";

echarts.use([
  BarChart,
  LineChart,
  DataZoomComponent,
  GridComponent,
  TooltipComponent,
  CanvasRenderer,
]);

export type ChartOption = echarts.EChartsCoreOption;
export type ChartInstance = echarts.EChartsType;

export function Chart({
  option,
  height = 320,
  onChartReady,
}: {
  option: ChartOption;
  height?: number;
  onChartReady?: (chart: ChartInstance) => void;
}) {
  const { theme } = useTheme();

  return (
    <ReactEChartsCore
      echarts={echarts}
      option={{ backgroundColor: "transparent", ...option }}
      theme={theme === "dark" ? "dark" : undefined}
      notMerge
      onChartReady={onChartReady}
      style={{ height }}
    />
  );
}
