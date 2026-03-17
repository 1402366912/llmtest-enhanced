import os
import json
import glob

# 配置路径
# 使用当前脚本所在目录作为根目录
ROOT_DIR = os.path.dirname(os.path.abspath(__file__))
OUTPUT_FILE = os.path.join(ROOT_DIR, "report.html")

def load_data():
    data = {"machines": [], "results": []}
    
    # 读取 machines.json
    machines_file = os.path.join(ROOT_DIR, "machines.json")
    if os.path.exists(machines_file):
        try:
            with open(machines_file, 'r', encoding='utf-8') as f:
                data["machines"] = json.load(f).get("machines", [])
        except Exception as e:
            print(f"Error reading machines.json: {e}")

    machine_map = {m["id"]: m["name"] for m in data["machines"]}

    # 遍历机器目录
    for machine_id in os.listdir(ROOT_DIR):
        machine_path = os.path.join(ROOT_DIR, machine_id)
        if not os.path.isdir(machine_path): continue
        
        machine_name = machine_map.get(machine_id, f"Machine-{machine_id}")
        
        # 读取 backends.json
        backends_file = os.path.join(machine_path, "backends.json")
        backend_map = {}
        if os.path.exists(backends_file):
            try:
                with open(backends_file, 'r', encoding='utf-8') as f:
                    backends = json.load(f).get("backends", [])
                    backend_map = {b["id"]: b["name"] for b in backends}
            except: pass

        # 遍历机器内的文件
        for item in os.listdir(machine_path):
            item_path = os.path.join(machine_path, item)
            if not os.path.isdir(item_path): continue

            results_dir = None
            b_id = "default"
            b_name = "Default"

            # 情况1: results 目录
            if item == "results":
                results_dir = item_path
            
            # 情况2: 后端目录
            elif item in backend_map:
                b_id = item
                b_name = backend_map[item]
                possible_results = os.path.join(item_path, "results")
                if os.path.exists(possible_results):
                    results_dir = possible_results
            
            # 处理结果目录
            if results_dir:
                for json_file in glob.glob(os.path.join(results_dir, "*.json")):
                    try:
                        with open(json_file, 'r', encoding='utf-8') as f:
                            content = json.load(f)
                            # 兼容不同格式
                            if not isinstance(content, dict): continue
                            
                            # 统一包装
                            items = content.items() if "ModelName" not in content else [("result", content)]

                            for key, res in items:
                                if not isinstance(res, dict) or "ModelName" not in res: continue
                                
                                data["results"].append({
                                    "machine_name": machine_name,
                                    "backend_name": b_name,
                                    "model_name": res.get("ModelName"),
                                    "concurrency": res.get("ConcurrencyLevel", 0),
                                    "context_tokens": res.get("ContextTargetTokens", 0),
                                    "tps": res.get("TokensPerSec", 0),
                                    "ttft_ms": (res.get("AvgFirstTokenLatency", 0) or 0) / 1e6,
                                    "latency_ms": (res.get("AvgLatency", 0) or 0) / 1e6,
                                    "success_rate": (res.get("SuccessRequests", 0) / (res.get("TotalRequests", 1) or 1)) * 100
                                })
                    except Exception as e:
                        print(f"Skipping {json_file}: {e}")
    return data

# HTML 模板
HTML_TEMPLATE = """
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>LLM Benchmark Report</title>
<script src="https://unpkg.com/vue@3/dist/vue.global.js"></script>
<script src="https://unpkg.com/echarts/dist/echarts.min.js"></script>
<style>
body { font-family: sans-serif; margin: 0; background: #f0f2f5; }
.header { background: #001529; color: white; padding: 15px 30px; display: flex; justify-content: space-between; align-items: center; }
.container { display: flex; height: calc(100vh - 60px); }
.sidebar { width: 300px; background: white; padding: 20px; border-right: 1px solid #ddd; overflow-y: auto; display: flex; flex-direction: column; gap: 20px; }
.main { flex: 1; padding: 20px; overflow-y: auto; }
.card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
.chart { height: 450px; width: 100%; }
.form-group label { display: block; margin-bottom: 8px; font-weight: bold; color: #333; }
select { width: 100%; padding: 10px; border: 1px solid #d9d9d9; border-radius: 4px; outline: none; transition: all 0.3s; }
select:focus { border-color: #40a9ff; box-shadow: 0 0 0 2px rgba(24,144,255,0.2); }
.stat-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 20px; }
.stat-box { background: white; padding: 20px; border-radius: 8px; text-align: center; box-shadow: 0 2px 8px rgba(0,0,0,0.05); }
.stat-value { font-size: 32px; font-weight: bold; color: #1890ff; margin-bottom: 5px; }
.stat-label { color: #888; font-size: 14px; }
h3 { margin-top: 0; color: #333; border-left: 4px solid #1890ff; padding-left: 10px; }
</style>
</head>
<body>
<div id="app">
    <div class="header">
        <h2 style="margin:0">LLM Benchmark Visualization</h2>
        <div style="font-size: 12px; opacity: 0.8">Generated by Script</div>
    </div>
    <div class="container">
        <div class="sidebar">
            <div class="form-group">
                <label>Machine</label>
                <select v-model="filters.machine">
                    <option value="">All Machines</option>
                    <option v-for="m in machines" :value="m">{{m}}</option>
                </select>
            </div>
            <div class="form-group">
                <label>Backend</label>
                <select v-model="filters.backend">
                    <option value="">All Backends</option>
                    <option v-for="b in backends" :value="b">{{b}}</option>
                </select>
            </div>
            <div class="form-group">
                <label>Model</label>
                <select v-model="filters.model">
                    <option value="">All Models</option>
                    <option v-for="m in models" :value="m">{{m}}</option>
                </select>
            </div>
            <div class="form-group">
                <label>Context Length</label>
                <select v-model="filters.context">
                    <option value="">All Contexts</option>
                    <option v-for="c in contexts" :value="c">{{c}} tokens</option>
                </select>
            </div>
            <div style="margin-top: auto; font-size: 12px; color: #999;">
                {{ filteredData.length }} records found
            </div>
        </div>
        <div class="main">
            <div class="stat-cards">
                <div class="stat-box">
                    <div class="stat-value">{{ filteredData.length }}</div>
                    <div class="stat-label">Total Tests</div>
                </div>
                <div class="stat-box">
                    <div class="stat-value">{{ maxTPS.toFixed(1) }}</div>
                    <div class="stat-label">Max TPS</div>
                </div>
                <div class="stat-box">
                    <div class="stat-value">{{ avgTTFT.toFixed(0) }} ms</div>
                    <div class="stat-label">Avg TTFT</div>
                </div>
            </div>

            <div class="card">
                <h3>TPS Leaderboard</h3>
                <div ref="chartTPS" class="chart"></div>
            </div>

            <div class="card">
                <h3>Concurrency Scaling (TPS vs Concurrency)</h3>
                <div ref="chartConcurrency" class="chart"></div>
            </div>
            
            <div class="card">
                <h3>Performance Scatter (TTFT vs Concurrency)</h3>
                <div ref="chartTTFT" class="chart"></div>
            </div>
        </div>
    </div>
</div>

<script>
const RAW_DATA = __DATA_PLACEHOLDER__;

const { createApp, ref, computed, onMounted, watch, nextTick } = Vue;

createApp({
    setup() {
        const data = ref(RAW_DATA.results);
        const filters = ref({ machine: "", backend: "", model: "", context: "" });
        
        const machines = computed(() => [...new Set(data.value.map(d => d.machine_name))].sort());
        const backends = computed(() => [...new Set(data.value.map(d => d.backend_name))].sort());
        const models = computed(() => [...new Set(data.value.map(d => d.model_name))].sort());
        const contexts = computed(() => [...new Set(data.value.map(d => d.context_tokens))].sort((a,b)=>a-b));

        const filteredData = computed(() => {
            return data.value.filter(d => {
                return (!filters.value.machine || d.machine_name === filters.value.machine) &&
                       (!filters.value.backend || d.backend_name === filters.value.backend) &&
                       (!filters.value.model || d.model_name === filters.value.model) &&
                       (!filters.value.context || d.context_tokens === filters.value.context);
            });
        });

        const maxTPS = computed(() => Math.max(...filteredData.value.map(d => d.tps), 0));
        const avgTTFT = computed(() => {
            const sum = filteredData.value.reduce((acc, d) => acc + d.ttft_ms, 0);
            return filteredData.value.length ? sum / filteredData.value.length : 0;
        });

        const chartTPS = ref(null);
        const chartConcurrency = ref(null);
        const chartTTFT = ref(null);
        let charts = {};

        const updateCharts = () => {
            if (!chartTPS.value) return;
            
            const subset = filteredData.value;
            if (subset.length === 0) return;
            
            // Chart 1: TPS Ranking
            const sorted = [...subset].sort((a,b) => a.tps - b.tps);
            const chart1 = echarts.init(chartTPS.value);
            chart1.setOption({
                tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
                grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
                xAxis: { type: 'value', name: 'TPS' },
                yAxis: { 
                    type: 'category', 
                    data: sorted.map(d => `${d.model_name} (c=${d.concurrency})`),
                    axisLabel: { width: 150, overflow: 'truncate' }
                },
                series: [{ 
                    type: 'bar', 
                    data: sorted.map(d => d.tps.toFixed(1)),
                    itemStyle: { color: '#1890ff' }
                }]
            });

            // Chart 2: Concurrency Scaling
            const groups = {};
            subset.forEach(d => {
                const key = `${d.machine_name} / ${d.backend_name} / ${d.model_name}`;
                if (!groups[key]) groups[key] = [];
                groups[key].push([d.concurrency, d.tps]);
            });
            const series = Object.keys(groups).map(k => ({
                name: k, type: 'line', smooth: true, symbolSize: 8,
                data: groups[k].sort((a,b) => a[0]-b[0])
            }));
            const chart2 = echarts.init(chartConcurrency.value);
            chart2.setOption({
                tooltip: { trigger: 'axis' },
                legend: { type: 'scroll', bottom: 0 },
                grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
                xAxis: { type: 'value', name: 'Concurrency' },
                yAxis: { type: 'value', name: 'TPS' },
                series: series
            });
            
            // Chart 3: TTFT Scatter
            const chart3 = echarts.init(chartTTFT.value);
            chart3.setOption({
                tooltip: { 
                    trigger: 'item',
                    formatter: p => `Concurrency: ${p.data[0]}<br>TTFT: ${p.data[1].toFixed(0)}ms<br>${p.seriesName}`
                },
                legend: { type: 'scroll', bottom: 0 },
                grid: { left: '3%', right: '4%', bottom: '10%', containLabel: true },
                xAxis: { type: 'value', name: 'Concurrency' },
                yAxis: { type: 'value', name: 'TTFT (ms)' },
                series: Object.keys(groups).map(k => ({
                    name: k, type: 'scatter', symbolSize: 10,
                    data: subset.filter(d => `${d.machine_name} / ${d.backend_name} / ${d.model_name}` === k)
                                .map(d => [d.concurrency, d.ttft_ms])
                }))
            });
            
            charts = { c1: chart1, c2: chart2, c3: chart3 };
        };

        watch(filteredData, () => { nextTick(updateCharts); });
        
        onMounted(() => {
            setTimeout(updateCharts, 500);
            window.addEventListener('resize', () => Object.values(charts).forEach(c => c.resize()));
        });

        return {
            filters, machines, backends, models, contexts, filteredData, maxTPS, avgTTFT,
            chartTPS, chartConcurrency, chartTTFT
        };
    }
}).mount('#app');
</script>
</body>
</html>
"""

def main():
    print("Loading data...")
    data = load_data()
    print(f"Loaded {len(data['results'])} test results.")
    
    html_content = HTML_TEMPLATE.replace("__DATA_PLACEHOLDER__", json.dumps(data))
    
    with open(OUTPUT_FILE, 'w', encoding='utf-8') as f:
        f.write(html_content)
    
    print(f"Report generated: {OUTPUT_FILE}")

if __name__ == "__main__":
    main()

