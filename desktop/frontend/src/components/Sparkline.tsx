/** Tiny line chart of recent total speed for the status bar. */
export function Sparkline({ values, width = 72, height = 14 }: { values: number[]; width?: number; height?: number }) {
  const max = Math.max(1, ...values);
  const n = values.length;
  if (n < 2) return <svg className="spark" width={width} height={height} aria-hidden="true" />;
  const pts = values
    .map((v, i) => `${((i / (n - 1)) * width).toFixed(1)},${(height - 1 - (v / max) * (height - 2)).toFixed(1)}`)
    .join(" ");
  return (
    <svg className="spark" width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden="true">
      <polyline points={`0,${height} ${pts} ${width},${height}`} className="spark-area" />
      <polyline points={pts} className="spark-line" />
    </svg>
  );
}
