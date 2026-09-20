package main

type WritingPrompt struct {
	ID           string `json:"id"`
	TaskType     string `json:"taskType"`     // task1, task2
	Category     string `json:"category"`     // table, line_graph, bar_chart, map, process, opinion, discussion, problem_solution, advantages
	Title        string `json:"title"`
	Prompt       string `json:"prompt"`
	ChartSVG     string `json:"chartSvg,omitempty"`  // Responsive inline SVG for Task 1
	DataTable    string `json:"dataTable,omitempty"` // Clean data summary
	ModelAnswer  string `json:"modelAnswer,omitempty"`
	BandGuidance string `json:"bandGuidance,omitempty"`
}

func getCategorizedTask1Prompts() []WritingPrompt {
	return []WritingPrompt{
		{
			ID:       "t1-bar-renewable",
			TaskType: "task1",
			Category: "bar_chart",
			Title:    "Renewable Energy Share by Region (2015 vs 2025)",
			Prompt:   "The bar chart illustrates the percentage of electricity generated from renewable sources across four global regions in 2015 and projected for 2025. Summarise the information by selecting and reporting the main features, and make comparisons where relevant.",
			ChartSVG: `<svg viewBox="0 0 600 320" xmlns="http://www.w3.org/2000/svg" class="ielts-task1-chart">
  <rect width="600" height="320" fill="#1e293b" rx="12"/>
  <text x="300" y="32" fill="#f8fafc" font-size="15" font-weight="bold" text-anchor="middle">Renewable Electricity Share (% of total) - 2015 vs 2025</text>
  <!-- Grid Lines -->
  <line x1="80" y1="250" x2="550" y2="250" stroke="#334155" stroke-width="1.5"/>
  <line x1="80" y1="200" x2="550" y2="200" stroke="#334155" stroke-dasharray="4"/>
  <line x1="80" y1="150" x2="550" y2="150" stroke="#334155" stroke-dasharray="4"/>
  <line x1="80" y1="100" x2="550" y2="100" stroke="#334155" stroke-dasharray="4"/>
  <line x1="80" y1="50" x2="550" y2="50" stroke="#334155" stroke-dasharray="4"/>
  <!-- Y Axis Labels -->
  <text x="70" y="255" fill="#94a3b8" font-size="11" text-anchor="end">0%</text>
  <text x="70" y="205" fill="#94a3b8" font-size="11" text-anchor="end">20%</text>
  <text x="70" y="155" fill="#94a3b8" font-size="11" text-anchor="end">40%</text>
  <text x="70" y="105" fill="#94a3b8" font-size="11" text-anchor="end">60%</text>
  <text x="70" y="55" fill="#94a3b8" font-size="11" text-anchor="end">80%</text>
  <!-- Bars: Europe (35% -> 68%) -->
  <rect x="110" y="162" width="36" height="88" fill="#38bdf8" rx="4"/>
  <text x="128" y="154" fill="#38bdf8" font-size="11" font-weight="bold" text-anchor="middle">35%</text>
  <rect x="152" y="80" width="36" height="170" fill="#10b981" rx="4"/>
  <text x="170" y="72" fill="#10b981" font-size="11" font-weight="bold" text-anchor="middle">68%</text>
  <text x="149" y="275" fill="#f1f5f9" font-size="12" text-anchor="middle">Europe</text>
  <!-- Bars: North America (22% -> 48%) -->
  <rect x="225" y="195" width="36" height="55" fill="#38bdf8" rx="4"/>
  <text x="243" y="187" fill="#38bdf8" font-size="11" font-weight="bold" text-anchor="middle">22%</text>
  <rect x="267" y="130" width="36" height="120" fill="#10b981" rx="4"/>
  <text x="285" y="122" fill="#10b981" font-size="11" font-weight="bold" text-anchor="middle">48%</text>
  <text x="264" y="275" fill="#f1f5f9" font-size="12" text-anchor="middle">N. America</text>
  <!-- Bars: Asia-Pacific (18% -> 42%) -->
  <rect x="340" y="205" width="36" height="45" fill="#38bdf8" rx="4"/>
  <text x="358" y="197" fill="#38bdf8" font-size="11" font-weight="bold" text-anchor="middle">18%</text>
  <rect x="382" y="145" width="36" height="105" fill="#10b981" rx="4"/>
  <text x="400" y="137" fill="#10b981" font-size="11" font-weight="bold" text-anchor="middle">42%</text>
  <text x="379" y="275" fill="#f1f5f9" font-size="12" text-anchor="middle">Asia-Pacific</text>
  <!-- Bars: Latin America (52% -> 74%) -->
  <rect x="455" y="120" width="36" height="130" fill="#38bdf8" rx="4"/>
  <text x="473" y="112" fill="#38bdf8" font-size="11" font-weight="bold" text-anchor="middle">52%</text>
  <rect x="497" y="65" width="36" height="185" fill="#10b981" rx="4"/>
  <text x="515" y="57" fill="#10b981" font-size="11" font-weight="bold" text-anchor="middle">74%</text>
  <text x="494" y="275" fill="#f1f5f9" font-size="12" text-anchor="middle">Latin America</text>
  <!-- Legend -->
  <rect x="210" y="298" width="14" height="14" fill="#38bdf8" rx="2"/>
  <text x="232" y="310" fill="#cbd5e1" font-size="12">2015 (Actual)</text>
  <rect x="330" y="298" width="14" height="14" fill="#10b981" rx="2"/>
  <text x="352" y="310" fill="#cbd5e1" font-size="12">2025 (Projected)</text>
</svg>`,
			DataTable:    "| Region | 2015 (%) | 2025 Projected (%) | Change (% pts) |\n| :--- | :---: | :---: | :---: |\n| Europe | 35% | 68% | +33% |\n| North America | 22% | 48% | +26% |\n| Asia-Pacific | 18% | 42% | +24% |\n| Latin America | 52% | 74% | +22% |",
			BandGuidance: "Write at least 150 words. Include a clear overview highlighting that all regions experience substantial growth, with Latin America remaining the highest overall and Europe recording the largest proportional increase.",
		},
		{
			ID:       "t1-line-temperature",
			TaskType: "task1",
			Category: "line_graph",
			Title:    "Global CO2 Concentration and Mean Temperature Anomaly (1990–2020)",
			Prompt:   "The line graph shows changes in global atmospheric carbon dioxide concentration (ppm) and global surface temperature anomalies (°C) above the pre-industrial average between 1990 and 2020. Summarise the information and make relevant comparisons.",
			ChartSVG: `<svg viewBox="0 0 600 320" xmlns="http://www.w3.org/2000/svg" class="ielts-task1-chart">
  <rect width="600" height="320" fill="#1e293b" rx="12"/>
  <text x="300" y="30" fill="#f8fafc" font-size="14" font-weight="bold" text-anchor="middle">Atmospheric CO2 (ppm) vs Global Temperature Anomaly (°C)</text>
  <!-- Axes -->
  <line x1="80" y1="240" x2="530" y2="240" stroke="#475569" stroke-width="1.5"/>
  <line x1="80" y1="60" x2="80" y2="240" stroke="#475569" stroke-width="1.5"/>
  <!-- Year ticks -->
  <text x="90" y="260" fill="#94a3b8" font-size="11" text-anchor="middle">1990</text>
  <text x="200" y="260" fill="#94a3b8" font-size="11" text-anchor="middle">2000</text>
  <text x="310" y="260" fill="#94a3b8" font-size="11" text-anchor="middle">2010</text>
  <text x="420" y="260" fill="#94a3b8" font-size="11" text-anchor="middle">2020</text>
  <!-- Left Axis (CO2: 350 to 420) -->
  <text x="70" y="240" fill="#38bdf8" font-size="11" text-anchor="end">350</text>
  <text x="70" y="180" fill="#38bdf8" font-size="11" text-anchor="end">370</text>
  <text x="70" y="120" fill="#38bdf8" font-size="11" text-anchor="end">390</text>
  <text x="70" y="65" fill="#38bdf8" font-size="11" text-anchor="end">415</text>
  <!-- CO2 Line (354 -> 369 -> 389 -> 414) -->
  <path d="M 90 230 L 200 185 L 310 125 L 420 65" fill="none" stroke="#38bdf8" stroke-width="3"/>
  <circle cx="90" cy="230" r="4" fill="#38bdf8"/>
  <circle cx="200" cy="185" r="4" fill="#38bdf8"/>
  <circle cx="310" cy="125" r="4" fill="#38bdf8"/>
  <circle cx="420" cy="65" r="4" fill="#38bdf8"/>
  <!-- Temp Line (+0.4C -> +0.6C -> +0.9C -> +1.25C) -->
  <path d="M 90 210 L 200 170 L 310 130 L 420 80" fill="none" stroke="#f43f5e" stroke-width="3" stroke-dasharray="6"/>
  <circle cx="90" cy="210" r="4" fill="#f43f5e"/>
  <circle cx="200" cy="170" r="4" fill="#f43f5e"/>
  <circle cx="310" cy="130" r="4" fill="#f43f5e"/>
  <circle cx="420" cy="80" r="4" fill="#f43f5e"/>
  <!-- Legend -->
  <line x1="160" y1="295" x2="190" y2="295" stroke="#38bdf8" stroke-width="3"/>
  <text x="198" y="299" fill="#38bdf8" font-size="12">CO2 Concentration (ppm)</text>
  <line x1="360" y1="295" x2="390" y2="295" stroke="#f43f5e" stroke-width="3" stroke-dasharray="5"/>
  <text x="398" y="299" fill="#f43f5e" font-size="12">Temperature Anomaly (+°C)</text>
</svg>`,
			DataTable:    "| Year | CO2 (ppm) | Temperature Anomaly (°C) |\n| :--- | :---: | :---: |\n| 1990 | 354 ppm | +0.40 °C |\n| 2000 | 369 ppm | +0.61 °C |\n| 2010 | 389 ppm | +0.92 °C |\n| 2020 | 414 ppm | +1.25 °C |",
			BandGuidance: "Highlight the strong direct upward correlation between the accelerating rise of carbon dioxide and the steady warming of average surface temperatures over the 30-year timeframe.",
		},
		{
			ID:       "t1-table-tourism",
			TaskType: "task1",
			Category: "table",
			Title:    "International Tourist Arrivals and Revenue in Five Countries (2018 vs 2023)",
			Prompt:   "The table shows international tourist arrivals (in millions) and total visitor expenditure (in billion USD) across five nations in 2018 and 2023. Summarise the key trends and comparisons.",
			DataTable: "| Country | 2018 Arrivals (M) | 2023 Arrivals (M) | 2018 Revenue ($B) | 2023 Revenue ($B) |\n| :--- | :---: | :---: | :---: | :---: |\n| France | 89.4 M | 100.0 M | $67.4 B | $73.5 B |\n| Spain | 82.8 M | 85.1 M | $73.8 B | $92.0 B |\n| United States | 79.7 M | 66.5 M | $214.7 B | $180.2 B |\n| Japan | 31.2 M | 25.1 M | $41.1 B | $36.8 B |\n| Australia | 9.2 M | 7.4 M | $31.8 B | $28.5 B |",
			BandGuidance: "State the overall pattern: France and Spain experienced visitor arrival growth, while the US, Japan, and Australia recorded declines. Note that the US generated by far the highest monetary revenue despite lower arrival volumes than France.",
		},
		{
			ID:       "t1-map-harbor",
			TaskType: "task1",
			Category: "map",
			Title:    "Coastal Harbor Transformation Plan (2000 vs Present)",
			Prompt:   "The two maps show changes in a coastal harbor town between 2000 and the present day. Summarise the primary infrastructural and spatial developments.",
			ChartSVG: `<svg viewBox="0 0 600 300" xmlns="http://www.w3.org/2000/svg" class="ielts-task1-chart">
  <rect width="600" height="300" fill="#1e293b" rx="12"/>
  <!-- Map 2000 -->
  <g transform="translate(20, 20)">
    <rect width="260" height="250" fill="#0f172a" rx="8" stroke="#334155"/>
    <text x="130" y="24" fill="#38bdf8" font-size="14" font-weight="bold" text-anchor="middle">Year 2000</text>
    <!-- Industrial docks -->
    <rect x="20" y="45" width="220" height="60" fill="#475569" rx="4"/>
    <text x="130" y="80" fill="#cbd5e1" font-size="12" text-anchor="middle">Commercial Fishing Wharves</text>
    <!-- Warehouse -->
    <rect x="20" y="120" width="100" height="50" fill="#334155" rx="4"/>
    <text x="70" y="150" fill="#94a3b8" font-size="11" text-anchor="middle">Warehouses</text>
    <!-- Public Beach -->
    <rect x="140" y="120" width="100" height="50" fill="#ca8a04" opacity="0.8" rx="4"/>
    <text x="190" y="150" fill="#fef08a" font-size="11" text-anchor="middle">Public Beach</text>
    <!-- Old Road -->
    <line x1="20" y1="200" x2="240" y2="200" stroke="#64748b" stroke-width="4"/>
    <text x="130" y="225" fill="#94a3b8" font-size="11" text-anchor="middle">Industrial Access Road</text>
  </g>
  <!-- Map Present -->
  <g transform="translate(320, 20)">
    <rect width="260" height="250" fill="#0f172a" rx="8" stroke="#334155"/>
    <text x="130" y="24" fill="#10b981" font-size="14" font-weight="bold" text-anchor="middle">Present Day</text>
    <!-- Marina -->
    <rect x="20" y="45" width="220" height="60" fill="#0284c7" rx="4"/>
    <text x="130" y="80" fill="#f0f9ff" font-size="12" font-weight="bold" text-anchor="middle">Marina & Waterfront Promenade</text>
    <!-- Hotel & Restaurants -->
    <rect x="20" y="120" width="100" height="50" fill="#059669" rx="4"/>
    <text x="70" y="150" fill="#ecfdf5" font-size="10" font-weight="bold" text-anchor="middle">Luxury Hotel & Dining</text>
    <!-- Expanded Beach -->
    <rect x="140" y="120" width="100" height="50" fill="#eab308" rx="4"/>
    <text x="190" y="150" fill="#422006" font-size="10" font-weight="bold" text-anchor="middle">Resort Beach Park</text>
    <!-- Pedestrian Boulevard -->
    <line x1="20" y1="200" x2="240" y2="200" stroke="#10b981" stroke-width="4"/>
    <text x="130" y="225" fill="#a7f3d0" font-size="11" text-anchor="middle">Pedestrian Boulevard & Tramway</text>
  </g>
</svg>`,
			BandGuidance: "Contrast the industrial/commercial character of 2000 with the modern tourism, residential, and recreational identity of the present day.",
		},
		{
			ID:       "t1-process-geothermal",
			TaskType: "task1",
			Category: "process",
			Title:    "Geothermal Power Generation and District Heating Cycle",
			Prompt:   "The diagram outlines the linear production cycle used by a geothermal plant to produce electricity and supply residential district heating. Summarise the stages in logical sequence.",
			ChartSVG: `<svg viewBox="0 0 600 280" xmlns="http://www.w3.org/2000/svg" class="ielts-task1-chart">
  <rect width="600" height="280" fill="#1e293b" rx="12"/>
  <text x="300" y="28" fill="#f8fafc" font-size="14" font-weight="bold" text-anchor="middle">Stages in Closed-Loop Geothermal Energy Extraction</text>
  <!-- Step 1: Subterranean Extraction -->
  <rect x="20" y="55" width="120" height="75" fill="#0f172a" stroke="#38bdf8" stroke-width="2" rx="6"/>
  <text x="80" y="80" fill="#38bdf8" font-size="11" font-weight="bold" text-anchor="middle">1. Deep Well</text>
  <text x="80" y="100" fill="#94a3b8" font-size="10" text-anchor="middle">Pressurized hot brine</text>
  <text x="80" y="115" fill="#94a3b8" font-size="10" text-anchor="middle">pumped to surface</text>
  <!-- Arrow 1 -->
  <line x1="140" y1="92" x2="165" y2="92" stroke="#38bdf8" stroke-width="2.5" marker-end="url(#arrow)"/>
  <!-- Step 2: Flash Chamber -->
  <rect x="170" y="55" width="120" height="75" fill="#0f172a" stroke="#f59e0b" stroke-width="2" rx="6"/>
  <text x="230" y="80" fill="#f59e0b" font-size="11" font-weight="bold" text-anchor="middle">2. Flash Separator</text>
  <text x="230" y="100" fill="#94a3b8" font-size="10" text-anchor="middle">High pressure drop</text>
  <text x="230" y="115" fill="#94a3b8" font-size="10" text-anchor="middle">produces dry steam</text>
  <!-- Arrow 2 -->
  <line x1="290" y1="92" x2="315" y2="92" stroke="#f59e0b" stroke-width="2.5"/>
  <!-- Step 3: Turbine & Generator -->
  <rect x="320" y="55" width="120" height="75" fill="#0f172a" stroke="#10b981" stroke-width="2" rx="6"/>
  <text x="380" y="80" fill="#10b981" font-size="11" font-weight="bold" text-anchor="middle">3. Turbine / Grid</text>
  <text x="380" y="100" fill="#94a3b8" font-size="10" text-anchor="middle">Steam spins turbine;</text>
  <text x="380" y="115" fill="#94a3b8" font-size="10" text-anchor="middle">electricity exported</text>
  <!-- Arrow 3 -->
  <line x1="440" y1="92" x2="465" y2="92" stroke="#10b981" stroke-width="2.5"/>
  <!-- Step 4: District Heat & Reinjection -->
  <rect x="470" y="55" width="110" height="75" fill="#0f172a" stroke="#ec4899" stroke-width="2" rx="6"/>
  <text x="525" y="80" fill="#ec4899" font-size="11" font-weight="bold" text-anchor="middle">4. Heat Exchanger</text>
  <text x="525" y="100" fill="#94a3b8" font-size="10" text-anchor="middle">Residual heat piped</text>
  <text x="525" y="115" fill="#94a3b8" font-size="10" text-anchor="middle">to city homes</text>
  <!-- Return Loop -->
  <path d="M 525 130 L 525 210 L 80 210 L 80 130" fill="none" stroke="#64748b" stroke-width="2" stroke-dasharray="5"/>
  <text x="300" y="235" fill="#94a3b8" font-size="11" text-anchor="middle">5. Condensed cold fluid reinjected underground to replenish reservoir</text>
</svg>`,
			BandGuidance: "Describe all five operational steps sequentially using accurate passive voice structures ('is extracted', 'is depressurised', 'is converted', 'is reinjected').",
		},
	}
}

func getCategorizedTask2Prompts() []WritingPrompt {
	return []WritingPrompt{
		{
			ID:           "t2-op-wealth-env",
			TaskType:     "task2",
			Category:     "opinion",
			Title:        "Economic Expansion vs Environmental Protection",
			Prompt:       "Some economists argue that continuous economic growth is essential to eradicate poverty worldwide, while environmentalists contend that sustained industrial expansion inevitably depletes finite natural resources. To what extent do you agree or disagree with the view that economic growth must be prioritized?",
			BandGuidance: "State a clear position in the introduction, maintain that stance throughout, support arguments with concrete illustrations, and address counterarguments logically.",
		},
		{
			ID:           "t2-disc-edu-purpose",
			TaskType:     "task2",
			Category:     "discussion",
			Title:        "Higher Education: Career Readiness vs Academic Intellectualism",
			Prompt:       "Some people believe that tertiary education should focus strictly on providing practical skills tailored to immediate employment demands. Others argue that universities must primarily foster theoretical inquiry, critical thinking, and fundamental sciences regardless of short-term market utility. Discuss both views and give your own opinion.",
			BandGuidance: "Give balanced, in-depth consideration to both perspectives before clearly asserting your reasoned personal stance.",
		},
		{
			ID:           "t2-ps-urban-congestion",
			TaskType:     "task2",
			Category:     "problem_solution",
			Title:        "Urban Traffic Congestion in Modern Megacities",
			Prompt:       "Traffic congestion in major metropolitan centers has escalated into a severe economic and environmental dilemma. What are the primary factors driving this issue, and what comprehensive measures can municipal governments implement to alleviate urban gridlock?",
			BandGuidance: "Detail at least two distinct structural causes (e.g. deficient public transit, car-centric zoning) and match them directly with actionable policy solutions.",
		},
		{
			ID:           "t2-ad-remote-work",
			TaskType:     "task2",
			Category:     "advantages",
			Title:        "The Global Transition Towards Telecommuting",
			Prompt:       "An unprecedented proportion of the modern workforce now operates remotely from home rather than in central corporate headquarters. Do the advantages of widespread remote working outweigh its potential disadvantages for employees and society as a whole?",
			BandGuidance: "Compare the benefits (work-life flexibility, reduced commute emissions) with drawbacks (social isolation, blurring of boundaries), and justify which side holds greater overall significance.",
		},
	}
}
