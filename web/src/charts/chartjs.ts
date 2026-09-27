// Chart.js with the zoom plugin, loaded only by the Charts page.
import { Chart, Legend, LinearScale, LineController, LineElement, PointElement, Tooltip } from 'chart.js';
import zoomPlugin from 'chartjs-plugin-zoom';

Chart.register(LineController, LineElement, PointElement, LinearScale, Tooltip, Legend, zoomPlugin);

export { Chart };
