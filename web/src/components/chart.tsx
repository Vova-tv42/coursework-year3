import ReactEChartsCore from "echarts-for-react/esm/core";
import { BarChart } from "echarts/charts";
import {
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from "echarts/components";
import * as echarts from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";

import { useTheme } from "@/components/theme-provider";

echarts.use([
  BarChart,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  CanvasRenderer,
]);

export type ChartOption = echarts.EChartsCoreOption;

export function Chart({
  option,
  height = 320,
}: {
  option: ChartOption;
  height?: number;
}) {
  const { theme } = useTheme();

  return (
    <ReactEChartsCore
      echarts={echarts}
      option={{ backgroundColor: "transparent", ...option }}
      theme={theme === "dark" ? "dark" : undefined}
      notMerge
      style={{ height }}
    />
  );
}
