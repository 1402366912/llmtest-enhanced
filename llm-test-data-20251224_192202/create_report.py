import os
import json
import glob
from datetime import datetime

# 目标目录
BASE_DIR = os.path.dirname(os.path.abspath(__file__))

def load_json(path):
    try:
        with open(path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except Exception as e:
        print(f"Error loading {path}: {e}")
        return None

def collect_data():
    all_records = []
    
    # 1. 读取 Machines
    machines_path = os.path.join(BASE_DIR, 'machines.json')
    machines_data = load_json(machines_path)
    
    if not machines_data:
        print("No machines.json found.")
        return []

    machines_map = {m['id']: m['name'] for m in machines_data.get('machines', [])}

    # 2. 遍历 Machine 目录
    for machine_id in machines_map.keys():
        machine_dir = os.path.join(BASE_DIR, machine_id)
        if not os.path.isdir(machine_dir):
            continue
            
        machine_name = machines_map[machine_id]
        
        # 3. 读取 Backends
        backends_path = os.path.join(machine_dir, 'backends.json')
        backends_data = load_json(backends_path)
        
        backends_map = {}
        if backends_data:
            backends_map = {b['id']: b['name'] for b in backends_data.get('backends', [])}
            
        # 4. 遍历 Backend 目录 (以及 machine 根目录下的 results)
        # 4a. 处理 Backend 子目录
        for backend_id, backend_name in backends_map.items():
            results_dir = os.path.join(machine_dir, backend_id, 'results')
            if os.path.isdir(results_dir):
                process_results_dir(results_dir, machine_id, machine_name, backend_id, backend_name, all_records)
        
        # 4b. 处理 Machine 根目录下的 results (旧数据格式或无后端归属)
        root_results_dir = os.path.join(machine_dir, 'results')
        if os.path.isdir(root_results_dir):
             process_results_dir(root_results_dir, machine_id, machine_name, "default", "Default", all_records)

    return all_records

def process_results_dir(directory, m_id, m_name, b_id, b_name, records):
    for filename in os.listdir(directory):
        if not filename.endswith('.json'):
            continue
            
        filepath = os.path.join(directory, filename)
        data = load_json(filepath)
        
        if not data:
            continue
            
        # data 可能是 map[string]*TestResult
        for key, result in data.items():
            if not isinstance(result, dict):
                continue
                
            # 提取关键指标
            latency_percentiles = result.get("LatencyPercentiles", {})
            
            record = {
                "id": f"{m_id}_{b_id}_{key}",
                "machine": m_name,
                "backend": b_name,
                "model": result.get("ModelName", "Unknown"),
                "concurrency": result.get("ConcurrencyLevel", 0),
                "context": result.get("ContextTargetTokens", 0),
                "tps": result.get("TokensPerSec", 0),
                "rps": result.get("RequestsPerSec", 0),
                "ttft": float(result.get("AvgFirstTokenLatency", 0)) / 1e6, # 纳秒转毫秒
                "latency": float(result.get("AvgLatency", 0)) / 1e9, # 纳秒转秒
                "avg_input_tokens": result.get("AvgInputTokens", 0),
                "avg_output_tokens": result.get("AvgOutputTokens", 0),
                "avg_total_tokens": result.get("AvgTotalTokens", 0),
                "p50": float(latency_percentiles.get("50", 0)) / 1e9,
                "p90": float(latency_percentiles.get("90", 0)) / 1e9,
                "p95": float(latency_percentiles.get("95", 0)) / 1e9,
                "p99": float(latency_percentiles.get("99", 0)) / 1e9,
                "total_requests": result.get("TotalRequests", 0),
                "success_requests": result.get("SuccessRequests", 0),
                "success_rate": 0
            }
            
            if record["total_requests"] > 0:
                record["success_rate"] = (record["success_requests"] / record["total_requests"]) * 100
                
            records.append(record)

def generate_html(data):
    json_data = json.dumps(data, ensure_ascii=False)
    
    html = f"""
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LLM 性能测试报告</title>
    <!-- 引入 Vue 3 -->
    <script src="https://unpkg.com/vue@3/dist/vue.global.js"></script>
    <!-- 引入 ECharts -->
    <script src="https://cdn.jsdelivr.net/npm/echarts@5.4.3/dist/echarts.min.js"></script>
    <!-- 引入 Remix Icon -->
    <link href="https://cdn.jsdelivr.net/npm/remixicon@3.5.0/fonts/remixicon.css" rel="stylesheet">
    <!-- 引入 Tailwind CSS -->
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap');

        body {{
            background: radial-gradient(circle at 10% 20%, rgb(69, 42, 30) 0%, rgb(30, 15, 10) 90%);
            color: #f1f5f9;
            font-family: 'Outfit', sans-serif;
            min-height: 100vh;
            overflow-x: hidden;
        }}

        /* Glassmorphism Common Styles */
        .glass-panel {{
            background: rgba(255, 255, 255, 0.05);
            backdrop-filter: blur(24px);
            -webkit-backdrop-filter: blur(24px);
            border: 1px solid rgba(255, 255, 255, 0.1);
            box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.36);
        }}
        
        .glass-card {{
            background: rgba(255, 255, 255, 0.08);
            border-radius: 24px;
            border: 1px solid rgba(255, 255, 255, 0.12);
            transition: transform 0.3s ease, background 0.3s ease;
        }}
        
        .glass-card:hover {{
            background: rgba(255, 255, 255, 0.12);
            transform: translateY(-2px);
        }}

        .glass-input {{
            background: rgba(0, 0, 0, 0.2);
            border: 1px solid rgba(255, 255, 255, 0.1);
            color: white;
            backdrop-filter: blur(10px);
        }}
        
        .glass-input:focus {{
            background: rgba(0, 0, 0, 0.3);
            border-color: rgba(255, 255, 255, 0.3);
            outline: none;
        }}

        /* Scrollbar */
        ::-webkit-scrollbar {{
            width: 8px;
            height: 8px;
        }}
        ::-webkit-scrollbar-track {{
            background: rgba(0, 0, 0, 0.1);
        }}
        ::-webkit-scrollbar-thumb {{
            background: rgba(255, 255, 255, 0.2);
            border-radius: 4px;
        }}
        ::-webkit-scrollbar-thumb:hover {{
            background: rgba(255, 255, 255, 0.3);
        }}

        /* Custom Utilities */
        .text-glow {{
            text-shadow: 0 0 20px rgba(255, 255, 255, 0.3);
        }}
        
        .sidebar-item {{
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        }}
        
        .sidebar-item.active {{
            background: rgba(255, 255, 255, 0.15);
            border-left: 3px solid #fbbf24;
        }}

        .table-row {{
            transition: background 0.2s;
        }}
        
        .table-row:hover {{
            background: rgba(255, 255, 255, 0.05);
        }}
    </style>
</head>
<body class="p-6 md:p-8 flex gap-8 h-screen overflow-hidden">
    
    <div id="app" class="w-full h-full flex gap-8 relative z-10">
        
        <!-- SIDEBAR -->
        <aside class="glass-panel w-64 h-full rounded-[30px] flex flex-col p-6 shrink-0 absolute md:relative transform -translate-x-full md:translate-x-0 transition-transform z-50" :class="{{ 'translate-x-0': showSidebar }}">
             <div class="flex items-center gap-3 mb-10 px-2">
                <div class="w-10 h-10 rounded-full bg-gradient-to-br from-amber-400 to-orange-600 flex items-center justify-center shadow-lg shadow-orange-500/30">
                    <i class="ri-bar-chart-box-fill text-xl text-white"></i>
                </div>
                <div>
                    <h1 class="font-bold text-lg tracking-wide">LLM Bench</h1>
                    <p class="text-[10px] text-white/50 tracking-wider uppercase">Analytics</p>
                </div>
            </div>

            <nav class="flex-1 space-y-2 overflow-y-auto pr-2 custom-scrollbar">
                <div class="text-[11px] font-bold text-white/40 uppercase tracking-widest mb-3 px-3">Filters</div>
                
                <div class="space-y-4">
                    <!-- Machine Filter -->
                    <div class="px-3">
                        <label class="block text-xs text-secondary mb-1.5 font-medium text-white/70">Machine</label>
                        <select v-model="filters.machine" class="glass-input w-full rounded-xl px-3 py-2 text-sm appearance-none cursor-pointer">
                            <option value="">All Machines</option>
                            <option v-for="m in uniqueMachines" :value="m">{{{{ m }}}}</option>
                        </select>
                    </div>

                    <!-- Backend Filter -->
                    <div class="px-3">
                         <label class="block text-xs text-secondary mb-1.5 font-medium text-white/70">Backend</label>
                        <select v-model="filters.backend" class="glass-input w-full rounded-xl px-3 py-2 text-sm appearance-none cursor-pointer">
                            <option value="">All Backends</option>
                            <option v-for="b in uniqueBackends" :value="b">{{{{ b }}}}</option>
                        </select>
                    </div>

                    <!-- Model Filter -->
                    <div class="px-3">
                         <label class="block text-xs text-secondary mb-1.5 font-medium text-white/70">Model</label>
                        <select v-model="filters.model" class="glass-input w-full rounded-xl px-3 py-2 text-sm appearance-none cursor-pointer">
                            <option value="">All Models</option>
                            <option v-for="m in uniqueModels" :value="m">{{{{ m }}}}</option>
                        </select>
                    </div>

                    <!-- Concurrency Filter -->
                    <div class="px-3">
                         <label class="block text-xs text-secondary mb-1.5 font-medium text-white/70">Concurrency</label>
                        <select v-model="filters.concurrency" class="glass-input w-full rounded-xl px-3 py-2 text-sm appearance-none cursor-pointer">
                            <option value="">All Levels</option>
                            <option v-for="c in uniqueConcurrency" :value="c">{{{{ c }}}}</option>
                        </select>
                    </div>
                </div>
                
                <div class="mt-8 px-3">
                     <button @click="resetFilters" class="w-full py-2.5 rounded-xl bg-white/10 hover:bg-white/20 text-xs font-medium transition-colors flex items-center justify-center gap-2 border border-white/5">
                        <i class="ri-refresh-line"></i> Reset Filters
                    </button>
                </div>
            </nav>

            <div class="mt-auto pt-6 border-t border-white/10 px-2">
                <div class="flex items-center gap-3 opacity-60 hover:opacity-100 transition-opacity cursor-pointer">
                    <div class="w-8 h-8 rounded-full bg-gradient-to-tr from-blue-400 to-indigo-500"></div>
                    <div class="text-xs">
                        <div class="font-medium">User Admin</div>
                        <div class="text-[10px] text-white/50">View Profile</div>
                    </div>
                </div>
            </div>
        </aside>

        <!-- MAIN CONTENT -->
        <main class="glass-panel flex-1 rounded-[30px] p-6 md:p-8 overflow-y-auto backdrop-blur-3xl relative flex flex-col">
            <!-- Mobile Toggle -->
            <button @click="showSidebar = !showSidebar" class="md:hidden absolute top-6 right-6 p-2 rounded-lg bg-white/10 text-white z-50">
                <i class="ri-menu-line"></i>
            </button>

            <!-- Header -->
            <header class="flex justify-between items-end mb-8">
                <div>
                    <h2 class="text-3xl font-light mb-1">Performance <span class="font-semibold text-amber-400">Report</span></h2>
                    <p class="text-sm text-white/50">Generated on {datetime.now().strftime('%Y-%m-%d %H:%M')}</p>
                </div>
                <div class="flex gap-4">
                     <div class="glass-card px-5 py-3 flex flex-col items-center min-w-[100px]">
                        <span class="text-2xl font-bold text-white">{{{{ filteredData.length }}}}</span>
                        <span class="text-[10px] text-white/40 uppercase tracking-wider">Test Cases</span>
                    </div>
                </div>
            </header>

            <!-- Charts Grid -->
             <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8 shrink-0">
                <div class="glass-card p-1 h-[600px] relative group">
                    <div class="absolute inset-0 bg-gradient-to-b from-white/5 to-transparent rounded-2xl pointer-events-none"></div>
                    <div ref="chartTps" class="w-full h-full"></div>
                </div>
                <div class="glass-card p-1 h-[600px] relative group">
                    <div class="absolute inset-0 bg-gradient-to-b from-white/5 to-transparent rounded-2xl pointer-events-none"></div>
                    <div ref="chartLatency" class="w-full h-full"></div>
                </div>
            </div>

            <!-- Table Section -->
            <div class="flex-1 glass-card overflow-hidden flex flex-col">
                <div class="px-6 py-4 border-b border-white/10 flex justify-between items-center bg-white/5">
                    <h3 class="font-medium text-lg flex items-center gap-2">
                        <i class="ri-table-2 text-amber-400"></i> Detailed Results
                    </h3>
                </div>
                <div class="flex-1 overflow-auto custom-scrollbar">
                    <table class="w-full text-left border-collapse text-xs">
                        <thead class="sticky top-0 bg-[#2d1b14] z-10 font-semibold text-white/80">
                            <tr>
                                <th class="px-4 py-3 border-b border-white/10">Model</th>
                                <th class="px-2 py-3 border-b border-white/10 text-center">Concurrency</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Success/Total</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Success Rate</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Avg Latency (s)</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Avg TTFT (ms)</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Avg Input</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Avg Output</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">Avg Total</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">RPS</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">TPS</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">P50 (s)</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">P90 (s)</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">P95 (s)</th>
                                <th class="px-2 py-3 border-b border-white/10 text-right">P99 (s)</th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-white/5">
                            <tr v-for="(row, index) in filteredData" :key="row.id" class="group hover:bg-white/5">
                                <td class="px-4 py-2 font-medium text-white/90 align-top">
                                    <div v-if="shouldShowModel(index)">{{{{ row.model }}}}</div>
                                </td>
                                <td class="px-2 py-2 text-center text-white/70">{{{{ row.concurrency }}}}</td>
                                <td class="px-2 py-2 text-right text-white/70">{{{{ row.success_requests }}}}/{{{{ row.total_requests }}}}</td>
                                <td class="px-2 py-2 text-right">
                                    <span :class="row.success_rate >= 99 ? 'text-emerald-400' : 'text-red-400'">{{{{ row.success_rate.toFixed(2) }}}}%</span>
                                </td>
                                <td class="px-2 py-2 text-right text-white/80">{{{{ row.latency.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right text-amber-400">{{{{ row.ttft.toFixed(0) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/60">{{{{ row.avg_input_tokens.toFixed(0) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/60">{{{{ row.avg_output_tokens.toFixed(0) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/60">{{{{ row.avg_total_tokens.toFixed(0) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/80">{{{{ row.rps.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right font-medium text-emerald-400">{{{{ row.tps.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/70">{{{{ row.p50.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/70">{{{{ row.p90.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/70">{{{{ row.p95.toFixed(2) }}}}</td>
                                <td class="px-2 py-2 text-right text-white/70">{{{{ row.p99.toFixed(2) }}}}</td>
                            </tr>
                             <tr v-if="filteredData.length === 0">
                                <td colspan="15" class="text-center py-12 text-white/30 italic">No matching records found</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

        </main>
        
        <!-- Decoration Orbs -->
        <div class="fixed top-[-10%] right-[-5%] w-[500px] h-[500px] rounded-full bg-amber-600/20 blur-[100px] pointer-events-none z-0"></div>
        <div class="fixed bottom-[-10%] left-[-5%] w-[600px] h-[600px] rounded-full bg-rose-900/20 blur-[120px] pointer-events-none z-0"></div>

    </div>

    <script>
        const rawData = {json_data};

        const {{ createApp, ref, computed, onMounted, watch }} = Vue;

        createApp({{
            setup() {{
                const data = ref(rawData);
                const filters = ref({{
                    machine: '',
                    backend: '',
                    model: '',
                    concurrency: ''
                }});
                const showSidebar = ref(false);

                const chartTpsRef = ref(null);
                const chartLatencyRef = ref(null);
                let myChartTps = null;
                let myChartLatency = null;

                const uniqueMachines = computed(() => [...new Set(data.value.map(d => d.machine))].sort());
                const uniqueBackends = computed(() => [...new Set(data.value.map(d => d.backend))].sort());
                const uniqueModels = computed(() => [...new Set(data.value.map(d => d.model))].sort());
                const uniqueConcurrency = computed(() => [...new Set(data.value.map(d => d.concurrency))].sort((a,b) => a-b));

                const filteredData = computed(() => {{
                    const result = data.value.filter(item => {{
                        return (!filters.value.machine || item.machine === filters.value.machine) &&
                               (!filters.value.backend || item.backend === filters.value.backend) &&
                               (!filters.value.model || item.model === filters.value.model) &&
                               (!filters.value.concurrency || item.concurrency == filters.value.concurrency);
                    }});
                    // Sort by Model then Concurrency for table
                    return result.sort((a, b) => {{
                        if (a.model === b.model) {{
                            return a.concurrency - b.concurrency;
                        }}
                        return a.model.localeCompare(b.model);
                    }});
                }});

                function resetFilters() {{
                    filters.value = {{ machine: '', backend: '', model: '', concurrency: '' }};
                }}

                const shouldShowModel = (index) => {{
                    if (index === 0) return true;
                    return filteredData.value[index].model !== filteredData.value[index - 1].model;
                }};

                function updateCharts() {{
                    if (!myChartTps || !myChartLatency) return;

                    // Sort by TPS for charts
                    const chartData = [...filteredData.value].sort((a, b) => b.tps - a.tps).slice(0, 15);
                    const xAxisData = chartData.map(d => d.model + '\\n' + d.concurrency + 'c');

                    const commonOption = {{
                        backgroundColor: 'transparent',
                        textStyle: {{ color: 'rgba(255,255,255,0.7)', fontFamily: 'Outfit' }},
                        tooltip: {{
                            trigger: 'axis',
                            backgroundColor: 'rgba(30, 30, 30, 0.8)',
                            borderColor: '#555',
                            textStyle: {{ color: '#fff' }}
                        }},
                        grid: {{ left: '3%', right: '4%', bottom: '3%', top: '15%', containLabel: true }},
                        xAxis: {{
                            type: 'category',
                            data: xAxisData,
                            axisLine: {{ show: false }},
                            axisTick: {{ show: false }},
                            // rotate: 45 to avoid overlap, interval: 0 to show all
                            axisLabel: {{ color: 'rgba(255,255,255,0.5)', interval: 0, rotate: 45, fontSize: 10 }}
                        }}
                    }};

                    // TPS Chart
                    myChartTps.setOption({{
                        ...commonOption,
                        title: {{ text: 'Top TPS Performance', left: 'center', textStyle: {{ color: '#fff', fontSize: 16, fontWeight: 500 }} }},
                        yAxis: {{
                            type: 'value',
                            splitLine: {{ lineStyle: {{ type: 'dashed', color: 'rgba(255,255,255,0.1)' }} }},
                            axisLabel: {{ color: 'rgba(255,255,255,0.5)' }}
                        }},
                        series: [{{
                            data: chartData.map(d => d.tps),
                            type: 'bar',
                            itemStyle: {{
                                color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                                    {{ offset: 0, color: '#34d399' }},
                                    {{ offset: 1, color: 'rgba(52, 211, 153, 0.1)' }}
                                ]),
                                borderRadius: [6, 6, 0, 0]
                            }},
                            label: {{ show: true, position: 'top', color: '#fff', formatter: (params) => Math.round(params.value) }}
                        }}]
                    }});

                    // Latency Chart
                    myChartLatency.setOption({{
                        ...commonOption,
                        title: {{ text: 'Latency & TTFT Analysis', left: 'center', textStyle: {{ color: '#fff', fontSize: 16, fontWeight: 500 }} }},
                        legend: {{ data: ['TTFT (ms)', 'Latency (s)'], bottom: 0, textStyle: {{ color: '#ccc' }} }},
                         yAxis: [
                            {{ type: 'value', name: 'TTFT', position: 'left', splitLine: {{ show: false }}, axisLabel: {{ color: '#fbbf24' }} }},
                            {{ type: 'value', name: 'Latency', position: 'right', splitLine: {{ lineStyle: {{ type: 'dashed', color: 'rgba(255,255,255,0.1)' }} }}, axisLabel: {{ color: '#93c5fd' }} }}
                        ],
                        series: [
                            {{
                                name: 'TTFT (ms)',
                                data: chartData.map(d => d.ttft),
                                type: 'bar',
                                yAxisIndex: 0,
                                itemStyle: {{ color: '#fbbf24', borderRadius: [4, 4, 0, 0] }},
                            }},
                            {{
                                name: 'Latency (s)',
                                data: chartData.map(d => d.latency),
                                type: 'line',
                                yAxisIndex: 1,
                                smooth: true,
                                symbolSize: 8,
                                itemStyle: {{ color: '#93c5fd' }},
                                lineStyle: {{ width: 3, shadowBlur: 10, shadowColor: 'rgba(147, 197, 253, 0.5)' }}
                            }}
                        ]
                    }});
                }}

                onMounted(() => {{
                    myChartTps = echarts.init(chartTpsRef.value);
                    myChartLatency = echarts.init(chartLatencyRef.value);
                    updateCharts();
                    
                    window.addEventListener('resize', () => {{
                        myChartTps.resize();
                        myChartLatency.resize();
                    }});
                }});

                watch(filteredData, () => updateCharts());

                return {{
                    filters, filteredData, uniqueMachines, uniqueBackends, uniqueModels, uniqueConcurrency,
                    resetFilters, chartTps: chartTpsRef, chartLatency: chartLatencyRef, showSidebar, shouldShowModel
                }};
            }}
        }}).mount('#app');
    </script>
</body>
</html>
    """
    
    output_path = os.path.join(BASE_DIR, 'report.html')
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(html)
    print(f"Report generated: {output_path}")

if __name__ == '__main__':
    data = collect_data()
    print(f"Collected {len(data)} records.")
    generate_html(data)

