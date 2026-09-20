package main

import (
	"fmt"
)

type readingStoryBuilder func(year int, entity, topic string, itemCode, val1, val2 int) (contextStr, evidence, prompt string, choices []string, explanation, tip string)

type listeningScenarioBuilder func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (dialogue, prompt string, choices []string, explanation string)

// Authentic reading stories with realistic academic themes, balanced distractors, and educational explanations
var readingStoryBuilders = []readingStoryBuilder{
	// 0: Marine Ecology & Whale Acoustic Communication
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		pct1 := 20 + (val1 % 30)
		pct2 := pct1 + 15
		evidence := fmt.Sprintf("acoustic analysis revealed that ambient shipping noise reduced the effective communication range of blue whales by %d percent in open waters and up to %d percent in confined coastal corridors", pct1, pct2)
		contextStr := fmt.Sprintf("A comprehensive longitudinal study conducted in %d by the marine research team at %s investigated cetacean acoustic behavior. Researchers deployed passive acoustic monitoring arrays along continental migration routes. The empirical findings demonstrated that %s. Consequently, conservation biologists advocated for the establishment of seasonal vessel speed limits to safeguard vital feeding and breeding habitats.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d investigation by %s on %s, what was the primary impact of commercial maritime traffic?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("It significantly curtailed the vocal transmission radius of blue whales by %d to %d percent", pct1, pct2),
			fmt.Sprintf("It forced whale populations to relocate permanently to inland freshwater estuaries"),
			fmt.Sprintf("It completely suppressed all vocalization attempts during nighttime migrations"),
			fmt.Sprintf("It had negligible ecological impact due to the whales' adaptive frequency modulation"),
		}
		explanation := fmt.Sprintf("Berdasarkan teks, kebisingan lalu lintas kapal terbukti memotong jangkauan komunikasi akustik paus biru sebesar %d%% hingga %d%%. Opsi kedua salah karena paus tidak pindah ke muara air tawar; opsi ketiga terlalu ekstrem ('completely suppressed'); dan opsi keempat bertentangan dengan bukti penelitian.", pct1, pct2)
		tip := "Perhatikan parafrase: 'curtailed the vocal transmission radius' adalah sinonim konteks dari 'reduced the effective communication range'."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 1: Urban Architecture & Permeable Materials
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		tempDrop := 3 + (val1 % 6)
		runoffRed := 40 + (val2 % 35)
		evidence := fmt.Sprintf("permeable pavement installations lowered localized surface temperatures by %d degrees Celsius and decreased stormwater runoff volume by %d percent", tempDrop, runoffRed)
		contextStr := fmt.Sprintf("In %d, urban climatologists and civil engineers at %s published a detailed evaluation of sustainable infrastructure in metropolitan districts. Field measurements across pilot sites confirmed that %s compared to traditional non-porous asphalt. The municipal planning authority subsequently mandated the integration of porous surfaces in all future commercial zone developments.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d sustainable infrastructure study by %s on %s, what dual benefit of permeable paving was documented?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("A %d°C surface cooling effect coupled with a %d percent reduction in stormwater runoff", tempDrop, runoffRed),
			fmt.Sprintf("Complete elimination of urban ambient heat alongside zero maintenance requirements"),
			fmt.Sprintf("Increased structural durability under heavy freight with higher rainwater retention on surfaces"),
			fmt.Sprintf("A modest reduction in paving construction costs without measurable thermal changes"),
		}
		explanation := fmt.Sprintf("Penelitian membuktikan dua manfaat terukur: penurunan suhu permukaan sebesar %d°C dan pengurangan limpasan air hujan sebesar %d%%. Opsi kedua terlalu berlebihan ('complete elimination'); opsi ketiga salah karena air justru diserap ke tanah bukan tertahan di permukaan; opsi keempat salah karena manfaat termal terbukti nyata.", tempDrop, runoffRed)
		tip := "Dalam soal yang menanyakan manfaat ganda (dual benefit), pastikan kedua klaim dalam opsi pilihan didukung oleh bukti teks."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 2: Ancient Roman Maritime Concrete & Mineral Crystallization
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := "the interaction between seawater and volcanic pozzolana triggered the post-curing growth of interlocking aluminous tobermorite crystals within the mortar"
		contextStr := fmt.Sprintf("Archaeological excavations led in %d by researchers from %s examined submerged breakwaters and harbor masonry constructed over two millennia ago. Petrographic micro-spectrometry confirmed that %s. Rather than deteriorating in aggressive marine environments, the chemical reaction actively reinforced the structural cohesion of the concrete over centuries.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d petrographic analysis by %s on %s, why did ancient Roman marine structures strengthen over time?", year, entity, topic)
		choices := []string{
			"Seawater chemically reacted with volcanic ash to form reinforcing crystal lattices inside the mortar",
			"Ancient builders coated the exterior surfaces with impermeable petroleum-based sealants",
			"The absence of internal minerals prevented corrosive chemical reactions beneath the waves",
			"Underwater seismic tremors compacted the lime mixture into denser metamorphic rock layers",
		}
		explanation := "Teks menjelaskan bahwa kontak air laut dengan abu vulkanik (pozzolana) memicu pertumbuhan kristal tobermorite yang saling mengunci sehingga memperkuat struktur semen seiring waktu. Opsi lain tidak berdasar pada teks ilmiah dan spekulatif."
		tip := "Identifikasi hubungan sebab-akibat (cause and effect) ilmiah: 'mineral growth triggered by seawater' -> 'actively reinforced structural cohesion'."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 3: Paleoclimatology & Ice Core Isotope Stratigraphy
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		pctRise := 120 + (val1 % 60)
		evidence := fmt.Sprintf("atmospheric carbon concentrations remained within a stable historical equilibrium before experiencing an unprecedented %d percent surge following rapid industrial expansion", pctRise)
		contextStr := fmt.Sprintf("During a deep glaciological drilling project concluded in %d, paleoclimatologists from %s extracted ice core samples spanning eight glacial cycles. Isotopic spectrometry of trapped ancient air bubbles verified that %s. These empirical baselines provide vital contextual data for calibrating modern predictive climate simulations.", year, entity, evidence)
		prompt := fmt.Sprintf("What key conclusion was drawn from the %d ice core investigation by %s regarding %s?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Pre-industrial carbon levels maintained equilibrium prior to an abrupt %d percent rise in modern eras", pctRise),
			"Ancient atmospheric greenhouse gases fluctuated unpredictably without any recognizable cyclical patterns",
			"Deep ice core strata experienced severe atmospheric gas contamination across all sampled layers",
			"Historical temperature variations occurred independently of atmospheric greenhouse gas concentrations",
		}
		explanation := fmt.Sprintf("Data isotop membuktikan kestabilan konsentrasi karbon pada era pra-industri sebelum terjadi lonjakan tajam sebesar %d%% di era modern. Opsi kedua, ketiga, dan keempat bertolak belakang dengan temuan ilmiah pada teks.", pctRise)
		tip := "Fokus pada kontras antara 'stable historical equilibrium' dan 'unprecedented surge' yang diparafrasekan dalam pilihan jawaban."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 4: Pediatric Nutrition & Micro-Encapsulated Micronutrients
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		rate := 25 + (val1 % 25)
		evidence := fmt.Sprintf("bioavailability assays indicated that lipid encapsulation enhanced zinc and iron absorption by %d percent compared to standard powdered supplements", rate)
		contextStr := fmt.Sprintf("A controlled clinical trial completed in %d by public health researchers at %s investigated interventions for childhood micronutrient deficiencies in rural schools. Longitudinal biomarker tracking established that %s without inducing common gastrointestinal side effects. The results demonstrated the superiority of lipid-based nutrient delivery systems in school dietary programs.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d nutritional clinical trial by %s on %s, what advantage was demonstrated by micro-encapsulated nutrients?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("They yielded a %d percent increase in mineral uptake while avoiding adverse digestive irritation", rate),
			"They lowered manufacturing costs substantially but required significantly higher daily dosage volumes",
			"They produced rapid immediate absorption but caused severe long-term biochemical resistance in subjects",
			"They performed identically to conventional powdered supplements in all tested age brackets",
		}
		explanation := fmt.Sprintf("Teks menyatakan enkapsulasi lipid meningkatkan penyerapan mineral sebesar %d%% tanpa efek samping pencernaan ('without inducing gastrointestinal side effects'). Pilihan lain memuat klaim keliru yang bertentangan dengan hasil uji klinis.", rate)
		tip := "Perhatikan frasa 'without inducing common gastrointestinal side effects' yang diparafrasekan menjadi 'avoiding adverse digestive irritation'."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 5: Renewable Energy & Hybrid Pumped Storage
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		eff := 75 + (val1 % 15)
		evidence := fmt.Sprintf("closed-loop pumped hydroelectric storage achieved a round-trip thermodynamic efficiency of %d percent during daily peak balancing cycles", eff)
		contextStr := fmt.Sprintf("In an energy engineering report published in %d, power systems analysts from %s examined grid integration strategies for intermittent wind and solar resources. Performance telemetry confirmed that %s. Grid operators highlighted that combining rapid-response batteries with large-scale pumped hydro provides the optimal trade-off between instant frequency regulation and multi-hour bulk capacity.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d power systems analysis by %s on %s, how do hybrid storage configurations optimize grid reliability?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("By pairing battery quick-response capabilities with high-capacity pumped hydro operating at %d percent efficiency", eff),
			"By relying exclusively on lithium batteries to eliminate the need for any mechanical water storage infrastructure",
			"By running conventional fossil fuel turbines continuously at full capacity to prevent grid voltage fluctuations",
			"By disconnecting renewable generation sources during unpredictable regional weather shifts",
		}
		explanation := fmt.Sprintf("Teks menjelaskan bahwa kombinasi baterai respons cepat dengan pumped hydro berkapasitas besar (efisiensi %d%%) memberikan solusi optimal untuk kestabilan listrik. Opsi lain merekomendasikan solusi yang tidak ramah lingkungan atau mengabaikan sistem hibrida.", eff)
		tip := "Cari ide pokok mengenai integrasi sistem hibrida ('combining rapid-response batteries with large-scale pumped hydro')."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 6: Cognitive Psychology & Bilingual Executive Function
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := "lifelong active bilingualism fosters greater neural pathway redundancy, allowing older adults to maintain executive cognitive functions despite localized age-related cerebral atrophy"
		contextStr := fmt.Sprintf("A comprehensive neuroimaging study published in %d by cognitive psychologists at %s evaluated executive control across diverse adult demographics. Quantitative fMRI and task-switching paradigms confirmed that %s. The authors concluded that continuous bilingual language management serves as a powerful protective factor reinforcing cognitive reserve throughout late adulthood.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d neuroimaging findings by %s on %s, how does bilingualism contribute to cognitive resilience?", year, entity, topic)
		choices := []string{
			"By developing compensatory neural pathways that preserve mental flexibility despite physical brain aging",
			"By accelerating the physical restoration of damaged brain tissue through auditory stimulation",
			"By eliminating the biological occurrence of neurodegenerative processes entirely in elderly populations",
			"By restricting linguistic processing exclusively to the dominant left cerebral hemisphere",
		}
		explanation := "Penelitian membuktikan bahwa kemampuan dwibahasa membangun jalur saraf alternatif (*neural pathway redundancy*) yang menjaga fungsi kognitif eksekutif dari penuaan otak. Opsi ketiga terlalu absolut (*eliminating entirely*); opsi kedua dan keempat tidak didukung fakta teks."
		tip := "Hati-hati dengan pilihan jawaban yang menggunakan kata mutlak seperti 'eliminating entirely'. Pilih parafrase yang proporsional ('preserve mental flexibility')."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 7: Agricultural Agroforestry & Soil Carbon Sequestration
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		pctCarbon := 30 + (val1 % 30)
		evidence := fmt.Sprintf("integrating multi-strata shade trees into coffee agroecosystems increased deep soil organic carbon stocks by %d percent over a five-year monitoring cycle", pctCarbon)
		contextStr := fmt.Sprintf("In %d, agricultural ecologists at %s completed a multi-year comparative assessment of tropical farming techniques. Soil core spectrometry established that %s while simultaneously enhancing mycorrhizal fungi biodiversity. The researchers demonstrated that diverse vegetative canopy layers reduce soil temperature stress and mitigate erosion during severe tropical precipitation.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d agroforestry evaluation by %s on %s, what ecological advantage was confirmed in shaded coffee farms?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("A %d percent enhancement in deep soil organic carbon alongside fungal biodiversity gains", pctCarbon),
			"Complete immunity to severe tropical rainstorms without any required soil drainage infrastructure",
			"Lower annual crop yields caused by excessive sunlight interception under dense canopy layers",
			"A total reliance on synthetic fertilizers to sustain mycorrhizal microbial communities",
		}
		explanation := fmt.Sprintf("Teks menyatakan sistem agroforestri pohon peneduh meningkatkan cadangan karbon organik tanah sebesar %d%% serta memperkaya keanekaragaman hayati jamur tanah. Opsi B terlalu berlebihan, C dan D kontradiktif.", pctCarbon)
		tip := "Perhatikan bahwa peningkatan karbon tanah dan keanekaragaman jamur disebutkan bersamaan dalam bukti kalimat teks."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 8: Linguistics & Digital Documentation of Endangered Dialects
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := "combining community-led oral storytelling recordings with interactive phonetic mobile tools increased generational language adoption among youth significantly more than passive text archives"
		contextStr := fmt.Sprintf("An applied anthropological report published in %d by field linguists from %s documented revitalization efforts across indigenous speech communities. Quantitative survey data demonstrated that %s. Community leaders emphasized that empowering young speakers to create digital audio narratives transformed the dialect from an academic relic into a living medium of daily expression.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d linguistic report by %s on %s, why was digital storytelling more effective for language preservation?", year, entity, topic)
		choices := []string{
			"It actively engaged younger generations through interactive multimedia rather than relying solely on static texts",
			"It standardized indigenous dialects into simplified phonetic formats for international academic publications",
			"It restricted oral communication practice strictly to formal classroom testing environments",
			"It replaced the role of community elders with automated synthetic speech generation software",
		}
		explanation := "Teks menegaskan bahwa pendekatan rekaman cerita lisan berbasis komunitas dan perangkat seluler interaktif terbukti lebih efektif membangkitkan minat generasi muda daripada arsip teks pasif. Opsi B, C, dan D bertentangan dengan filosofi pelestarian budaya komunitas."
		tip := "Fokus pada perbandingan: 'interactive phonetic mobile tools' vs 'passive text archives'."
		return contextStr, evidence, prompt, choices, explanation, tip
	},

	// 9: Astronomy & Exoplanetary Biosignature Spectroscopy
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := "transmission spectroscopy detected chemical disequilibrium characterized by coexisting atmospheric methane and water vapor in the habitable zone exoplanet's atmosphere"
		contextStr := fmt.Sprintf("In %d, an international astrophysical collaboration involving researchers from %s published high-resolution spectroscopic observations of a terrestrial exoplanet. Space telescope telemetry confirmed that %s. While not definitive proof of biological activity, astrobiologists underscored that such chemical disequilibrium represents one of the strongest candidate biosignatures currently detectable by modern orbital observatories.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d exoplanet atmosphere study by %s on %s, what finding generated significant interest among astrobiologists?", year, entity, topic)
		choices := []string{
			"The simultaneous presence of methane and water vapor indicating a state of atmospheric chemical disequilibrium",
			"Definitive and indisputable verification of macroscopic biological organisms inhabiting the planetary surface",
			"Complete absence of any atmospheric gases surrounding the rocky core of the habitable zone planet",
			"Direct photographic imagery showing artificial light emissions across planetary continental landmasses",
		}
		explanation := "Spektroskopi mendeteksi ketidakseimbangan kimiawi melalui keberadaan metana dan uap air secara bersamaan di atmosfer planet. Opsi B salah karena teks menegaskan temuan ini bukan bukti mutlak ('not definitive proof'); Opsi C dan D sepenuhnya salah."
		tip := "Perhatikan batasan ilmiah: 'not definitive proof of biology' tetapi merupakan 'strong candidate biosignature' (chemical disequilibrium)."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
}

// Generate an authentic reading question builder dynamically if index >= len(base)
func getReadingStoryBuilder(idx int) readingStoryBuilder {
	if idx < len(readingStoryBuilders) {
		return readingStoryBuilders[idx]
	}

	// Dynamic authentic themes
	domains := []struct {
		field, subject, outcome, distractor1, distractor2, distractor3 string
	}{
		{
			"Glaciology", "subglacial meltwater drainage networks",
			"accelerated basal sliding and seasonal ice velocity fluctuations",
			"halted glacier movement completely during summer warming",
			"crystallized into solid bedrock under extreme pressure",
			"evaporated through surface crevasses before reaching the bed",
		},
		{
			"Biomedical Engineering", "biodegradable polymeric cardiovascular stents",
			"facilitated localized vessel healing before safely dissolving without arterial inflammation",
			"caused permanent synthetic buildup in adjacent circulatory tissues",
			"remained permanently rigid within the arterial wall structure",
			"triggered acute immune rejection in the majority of patient cohorts",
		},
		{
			"Environmental Chemistry", "fungal bioremediation of industrial polycyclic hydrocarbons",
			"metabolized toxic soil contaminants into inert organic byproducts within six weeks",
			"sterilized all beneficial microorganisms residing in the surrounding soil profile",
			"concentrated toxic residues into volatile airborne aerosols",
			"exhibited zero biochemical activity under non-laboratory field conditions",
		},
		{
			"Urban Ecology", "vertical green facades on commercial high-rises",
			"captured ambient fine particulate matter and lowered indoor air conditioning energy demand",
			"increased external wall solar heat absorption during summer months",
			"attracted structural insect pests that degraded exterior concrete masonry",
			"blocked natural indoor daylight without providing any microclimate benefits",
		},
		{
			"Renewable Materials", "mycelium-based composite thermal insulation panels",
			"provided equivalent flame resistance and acoustic dampening to petroleum foams with zero toxic off-gassing",
			"deteriorated structurally upon initial contact with normal atmospheric humidity",
			"emitted higher levels of volatile organic compounds than synthetic fiberglass",
			"required expensive fossil-derived chemical binders to maintain structural density",
		},
		{
			"Aeroelastic Design", "flexible wingtip extensions inspired by raptor feathers",
			"suppressed turbulent induced drag and improved long-distance fuel economy during cruising",
			"caused dangerous structural resonance that forced pilots to lower flight altitudes",
			"increased aerodynamic resistance and elevated overall aircraft fuel consumption",
			"demanded continuous manual recalibration by flight control computers",
		},
	}

	d := domains[idx%len(domains)]
	return func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		pct := 15 + (val1 % 35)
		evidence := fmt.Sprintf("field measurements demonstrated that the evaluated approach %s, achieving an estimated %d percent performance improvement over legacy methods", d.outcome, pct)
		contextStr := fmt.Sprintf("An extensive technical evaluation conducted in %d by the research division at %s analyzed recent developments in %s. Rigorous experimental trials established the following result: %s. The multidisciplinary team concluded that implementing these sustainable methodologies offers substantial operational and ecological benefits across the sector.", year, entity, d.field, evidence)
		prompt := fmt.Sprintf("According to the %d investigation by %s concerning %s, what primary advantage was verified?", year, entity, d.field)
		choices := []string{
			fmt.Sprintf("It %s, delivering a %d percent measurable improvement", d.outcome, pct),
			fmt.Sprintf("It %s, requiring researchers to abort trials", d.distractor1),
			fmt.Sprintf("It %s throughout the entire experimental period", d.distractor2),
			fmt.Sprintf("It %s under realistic ambient conditions", d.distractor3),
		}
		explanation := fmt.Sprintf("Hasil eksperimen membuktikan bahwa metode baru ini %s (peningkatan efisiensi %d%%). Tiga pilihan lainnya merupakan distraktor yang mendeskripsikan kegagalan atau efek negatif yang bertentangan dengan temuan teks.", d.outcome, pct)
		tip := "Temukan kata kunci tindakan pada hasil pengujian ('field measurements demonstrated that...') dan cocokkan dengan klaim positif dalam pilihan jawaban."
		return contextStr, evidence, prompt, choices, explanation, tip
	}
}

// Authentic transactional listening scenarios with realistic conversational dynamics and plausible distractor traps
var listeningScenarioBuilders = []listeningScenarioBuilder{
	// Scenario 0: University Library Archival Room & Special Collections
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Student: Hello, I'm inquiring about booking the Special Collections archive room for my dissertation research on historical manuscripts.\nLibrarian: Welcome! Yes, the Rare Manuscripts Reading Room is on the third floor in Room %d.\nStudent: Great. Are there specific time slots available tomorrow morning?\nLibrarian: We have sessions starting at %d:00, but our curator orientation begins promptly at %d:%02d. You must attend the %d-minute handling briefing first.\nStudent: Understood. Is there any registration fee for postgraduate students?\nLibrarian: Access is entirely free with your student ID, though replacing a lost archive access lanyard costs %d pounds.\nStudent: Excellent, please book me for the %d:%02d orientation in Room %d.",
			roomNum, hour-1, hour, minute, 15, fee, hour, minute, roomNum)
		prompt := "At what time must the student arrive for the mandatory archive orientation briefing?"
		choices := []string{
			fmt.Sprintf("At %d:%02d in Room %d", hour, minute, roomNum),
			fmt.Sprintf("At %d:00 on the second floor", hour-1),
			fmt.Sprintf("At %d:%02d in the main entrance foyer", hour+1, minute),
			fmt.Sprintf("At 09:00 following payment of the %d pound fee", fee),
		}
		explanation := fmt.Sprintf("Pustakawan mengonfirmasi bahwa sesi orientasi wajib dimulai tepat pada pukul %d:%02d di Ruang %d. Pukul %d:00 adalah waktu awal sesi umum (distraktor), dan biaya %d pound hanya berlaku jika tali tanda pengenal hilang (bukan biaya masuk).", hour, minute, roomNum, hour-1, fee)
		return dialogue, prompt, choices, explanation
	},

	// Scenario 1: City Council Green Waste & Bulky Item Collection
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		pickupFee := 20 + (fee % 25)
		dialogue := fmt.Sprintf("Resident: Good morning. I'm calling the municipal environmental depot to arrange a bulky garden waste collection.\nOfficer: Certainly. Our standard residential service collects up to five bundled items. The scheduled collection date for your northern district is this Thursday at %d:%02d in the morning.\nResident: What is the processing fee for this collection?\nOfficer: Standard household bin pickups are covered by municipal taxes, but special bulky garden collections carry a subsidized charge of %d pounds.\nResident: Can I pay cash on the collection day?\nOfficer: No, payments must be processed online or over the phone prior to Thursday. Your reference code is North-%d.\nResident: Perfect, I will settle the %d pounds online today.",
			hour, minute, pickupFee, roomNum+100, pickupFee)
		prompt := "What is the requirement regarding the payment for the bulky waste collection?"
		choices := []string{
			fmt.Sprintf("The %d pound charge must be settled in advance via online or phone payment", pickupFee),
			"The payment should be handed directly to the truck driver in cash upon collection",
			"No payment is required as all bulky collections are funded fully by regular taxes",
			fmt.Sprintf("A %d pound penalty is charged if items are placed outside before noon", pickupFee+15),
		}
		explanation := fmt.Sprintf("Petugas menegaskan bahwa biaya %d pound tidak boleh dibayar tunai saat pengambilan, melainkan wajib dibayar di muka secara online atau via telepon sebelum hari Kamis.", pickupFee)
		return dialogue, prompt, choices, explanation
	},

	// Scenario 2: Community Sports Complex Induction & Locker Pass
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		deposit := 10 + (fee % 15)
		dialogue := fmt.Sprintf("Instructor: Welcome to the Riverside Sports Centre. Are you here for the fitness suite safety induction?\nMember: Yes, I booked the %d:%02d session in Studio %d.\nInstructor: Excellent. Before we begin, please note that all gym lockers require an electronic key token. A refundable deposit of %d pounds is required at the reception desk.\nMember: Do I get that %d pounds back when I return the token at the end of the year?\nInstructor: Yes, the full deposit is refunded as long as the token is returned undamaged. Let's head into Studio %d now.",
			hour, minute, roomNum, deposit, deposit, roomNum)
		prompt := "What did the instructor state regarding the electronic locker key token?"
		choices := []string{
			fmt.Sprintf("It requires a %d pound deposit which is fully refundable upon return", deposit),
			fmt.Sprintf("It is issued permanently for a non-refundable annual fee of %d pounds", deposit*2),
			"It must be purchased from an external sports equipment retailer",
			"It is only available to competitive athletes training in Studio 1",
		}
		explanation := fmt.Sprintf("Instruktur menjelaskan bahwa token kunci loker membutuhkan deposit %d pound yang dapat dikembalikan penuh (*fully refundable*) saat dikembalikan tanpa kerusakan.", deposit)
		return dialogue, prompt, choices, explanation
	},

	// Scenario 3: Heritage Castle Guided Tour & Headset Audio Receivers
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Visitor: Excuse me, does our general castle admission ticket include the audio guide tour?\nGuide: General admission includes entry to the courtyard and keep, but the multi-language audio tour headsets are collected here at Gatehouse %d.\nVisitor: How long does the full historical audio commentary take?\nGuide: The standard route takes %d minutes, and the extended architectural tour takes approximately 75 minutes. The afternoon departure leaves at %d:%02d.\nVisitor: Wonderful, we will join the %d-minute tour leaving from Gatehouse %d.",
			roomNum, 45, hour, minute, 45, roomNum)
		prompt := "Where should visitors collect their audio tour headsets?"
		choices := []string{
			fmt.Sprintf("At Gatehouse %d near the entrance", roomNum),
			"At the central keep souvenir shop",
			"Directly from the coach parking steward",
			"Inside the castle chapel after the general briefing",
		}
		explanation := fmt.Sprintf("Pemandu wisata menyatakan secara jelas bahwa headset audio tour dapat diambil di Gatehouse %d.", roomNum)
		return dialogue, prompt, choices, explanation
	},

	// Scenario 4: Academic Conference Workshop & Breakout Room Allocation
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Delegate: Hi, I'm registered for the Applied Linguistics panel on machine translation ethics.\nOrganizer: Welcome! That session was originally scheduled for the main auditorium, but due to equipment setup, it has been moved to Seminar Room %d in the West Wing.\nDelegate: Thank you for letting me know. What time does the panel discussion start?\nOrganizer: The panel commences at %d:%02d sharp, followed by audience Q&A until 11:30.\nDelegate: Great, I'll head over to Seminar Room %d now so I can get a seat.",
			roomNum, hour, minute, roomNum)
		prompt := "What change to the conference schedule was announced by the organizer?"
		choices := []string{
			fmt.Sprintf("The session venue was relocated to Seminar Room %d in the West Wing", roomNum),
			"The panel discussion was postponed until the following morning",
			"The presentation topic was changed from machine translation to data privacy",
			"The audience question-and-answer period was cancelled due to time limits",
		}
		explanation := fmt.Sprintf("Penyelenggara mengumumkan bahwa ruangan diskusi dipindahkan ke Seminar Room %d di West Wing karena penyesuaian peralatan.", roomNum)
		return dialogue, prompt, choices, explanation
	},
}

// Generate dynamic listening scenarios if index >= len(base)
func getListeningScenarioBuilder(idx int) listeningScenarioBuilder {
	if idx < len(listeningScenarioBuilders) {
		return listeningScenarioBuilders[idx]
	}

	dialogueContexts := []struct {
		r1, r2, rolePlace, topicName string
	}{
		{"Student Advisor", "Undergraduate", "Academic Counselling Office", "elective course prerequisites"},
		{"Laboratory Technician", "Research Assistant", "Microscopy Facility", "laser spectrometer calibration booking"},
		{"Career Consultant", "Job Candidate", "University Placement Centre", "mock interview scheduling"},
		{"Housing Officer", "International Student", "University Accommodation Office", "residence hall lease agreements"},
		{"Workshop Facilitator", "Participant", "Digital Skills Hub", "data visualization software training"},
		{"Customer Service Agent", "Passenger", "High-Speed Rail Information Desk", "season travel pass renewal"},
	}

	dc := dialogueContexts[idx%len(dialogueContexts)]
	return func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		cost := 15 + (fee % 35)
		dialogue := fmt.Sprintf("%s: Good day. Welcome to the %s. Are you inquiring about %s?\n%s: Yes, I wanted to verify the requirements and schedule for the upcoming session.\n%s: Certainly. The session takes place in Room %d at %d:%02d.\n%s: Is there any preparatory material I need to review in advance?\n%s: Yes, please download the introductory briefing packet from the portal. The materials are complimentary, but optional printed handbook copies cost %d pounds at the desk.\n%s: Understood. I will download the digital packet and arrive at Room %d by %d:%02d.",
			dc.r1, dc.rolePlace, dc.topicName,
			dc.r2,
			dc.r1, roomNum, hour, minute,
			dc.r2,
			dc.r1, cost,
			dc.r2, roomNum, hour, minute)
		prompt := fmt.Sprintf("According to the %s at the %s, what should participants do before attending the %d:%02d session?", dc.r1, dc.rolePlace, hour, minute)
		choices := []string{
			"Download the complimentary briefing packet from the online portal in advance",
			fmt.Sprintf("Pay the mandatory %d pound entrance charge at the reception counter", cost),
			"Submit a printed copy of their dissertation draft to Room 101",
			"Purchase commercial software licenses from an external vendor",
		}
		explanation := fmt.Sprintf("Petugas menjelaskan bahwa peserta disarankan mengunduh paket materi pengantar gratis dari portal sebelum datang ke Ruang %d pada pukul %d:%02d. Biaya %d pound hanya bersifat opsional jika menginginkan buku cetak.", roomNum, hour, minute, cost)
		return dialogue, prompt, choices, explanation
	}
}
