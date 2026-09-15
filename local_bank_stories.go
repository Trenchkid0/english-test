package main

import (
	"fmt"
)

type readingStoryBuilder func(year int, entity, topic string, itemCode, val1, val2 int) (contextStr, evidence, prompt string, choices []string, explanation, tip string)

type listeningScenarioBuilder func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (dialogue, prompt string, choices []string, explanation string)

var readingStoryBuilders = []readingStoryBuilder{
	// 0
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("hydrophones registered %d vocalization pulses from migrating blue whale pods", val1)
		contextStr := fmt.Sprintf("During the %d Antarctic expedition, marine researchers from %s deployed deep-water hydrophones along the continental shelf. Acoustic telemetry verified that %s through ice corridors. Climatologists concluded that seasonal pack-ice retreat directly alters cetacean communication channels across regional feeding basins.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d polar study by %s on %s, what acoustic data was confirmed?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Hydrophones registered %d vocalization pulses from blue whale pods", val1),
			"Acoustic pulses ceased entirely during early sea ice formation",
			"Whale pods permanently abandoned their southern polar migration corridors",
			"Hydrophone sensors suffered fatal transducer malfunctions under ice cover",
		}
		explanation := fmt.Sprintf("Teks menyatakan bahwa telemetri hydrophone mencatat %d pulsa vokalisasi paus biru.", val1)
		tip := "Cari data spesifik jumlah sinyal akustik yang terekam pada teks."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 1
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("surface temperatures decreased by %d degrees Celsius relative to conventional asphalt", val1)
		contextStr := fmt.Sprintf("In an urban climatology survey concluded in %d, architects at %s evaluated permeable concrete pavement across municipal intersections. High-resolution thermography confirmed that %s during peak solar exposure. The municipal board subsequently incorporated porous surfaces into the master zoning code.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d urban climate investigation by %s on %s, what thermal change was documented?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Porous surfaces achieved a %d degrees Celsius temperature reduction", val1),
			"Surface temperatures climbed sharply throughout midday sunlight",
			"Porous concrete collapsed structurally under commercial bus loads",
			"Urban microclimates showed zero sensitivity to pavement reflectivity",
		}
		explanation := fmt.Sprintf("Hasil termografi menunjukkan penurunan suhu sebesar %d derajat Celsius.", val1)
		tip := "Perhatikan angka penurunan temperatur pada permukaan berpori."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 2
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("volcanic pozzolana content reached %d percent in the foundation lime mix", val1)
		contextStr := fmt.Sprintf("Archaeological excavations undertaken in %d by %s examined Roman harbor masonry vaults. Petrographic spectrometry revealed that %s, enabling underwater chemical crystallization. This mineral synergy explains the exceptional seismic durability of ancient maritime fortifications.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d archaeological analysis by %s on %s, what was the measured pozzolana ratio?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Foundation mortar contained %d percent volcanic pozzolana", val1),
			"Pozzolana additives were absent from all subterranean maritime foundations",
			"Mortar durability deteriorated rapidly when submerged in saline water",
			"Ancient lime mixes relied entirely on crushed limestone aggregate",
		}
		explanation := fmt.Sprintf("Analisis petrografi membuktikan campuran abu pozzolana mencapai %d persen.", val1)
		tip := "Perhatikan data persentase komposisi mineral pada adukan semen kuno."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 3
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("pre-industrial methane levels remained below %d parts per billion", val1)
		contextStr := fmt.Sprintf("A deep ice core extracted in %d by paleoclimatologists at %s yielded atmospheric bubbles spanning centuries. High-precision spectrometry proved that %s prior to widespread industrialization. These findings provide vital baseline metrics for contemporary climate models.", year, entity, evidence)
		prompt := fmt.Sprintf("What atmospheric threshold was verified in the %d ice-core analysis by %s on %s?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Pre-industrial methane concentrations remained below %d parts per billion", val1),
			"Methane levels fluctuated erratically throughout the historical period",
			"Air bubbles trapped in deep ice strata had leaked into ambient fissures",
			"Ice core layers showed zero chemical differentiation across eras",
		}
		explanation := fmt.Sprintf("Spektrometri membuktikan konsentrasi metana pra-industri tetap di bawah %d ppb.", val1)
		tip := "Cari batas ambang konsentrasi metana dalam satuan parts per billion."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 4
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("micronutrient absorption rates increased by %d percent among participating children", val1)
		contextStr := fmt.Sprintf("In a randomized trial concluded in %d, pediatric researchers from %s evaluated micro-encapsulated zinc supplementation across primary schools. Serum biomarker assays established that %s over six months. The trial confirmed the efficacy of food fortification strategies in preventing childhood deficiency.", year, entity, evidence)
		prompt := fmt.Sprintf("In the %d pediatric nutritional study by %s on %s, what clinical improvement was observed?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Childhood micronutrient absorption improved by %d percent", val1),
			"Zinc supplementation produced adverse gastrointestinal intolerance in all cohorts",
			"Serum zinc levels remained identical to the control group",
			"Biomarker assays failed to capture any systemic mineral change",
		}
		explanation := fmt.Sprintf("Uji klinis mencatat kenaikan laju penyerapan mikronutrien sebesar %d persen.", val1)
		tip := "Temukan persentase perbaikan absorbsi gizi pada subjek anak-anak."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 5
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("regenerative braking recaptured %d megawatt-hours of kinetic energy", val1)
		contextStr := fmt.Sprintf("Rail transit engineers from %s conducted bench trials in %d on autonomous electric locomotive braking circuits. Telemetry logs demonstrated that %s during scheduled decelerations. The captured power was redirected to station auxiliary networks to reduce grid dependency.", entity, year, evidence)
		prompt := fmt.Sprintf("According to the %d rail engineering report by %s on %s, how much energy was reclaimed?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Braking systems recaptured %d megawatt-hours of kinetic energy", val1),
			"Inverter circuits overheated and tripped under high braking loads",
			"Autonomous braking consumed more grid electricity than friction braking",
			"Dynamometer sensors recorded zero kinetic energy recapture during trials",
		}
		explanation := fmt.Sprintf("Log telemetri membuktikan pemulihan energi sebesar %d megawatt-hours.", val1)
		tip := "Cari total energi terbarukan yang dipulihkan pada sistem pengereman kereta."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 6
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("beneficial mycorrhizal fungal biomass was %d percent higher in undisturbed soils", val1)
		contextStr := fmt.Sprintf("Soil metagenomics research performed in %d by agronomists at %s compared conservation agriculture with intensive tillage. DNA sequencing confirmed that %s under no-till management. This enhanced fungal network contributed directly to moisture retention during severe droughts.", year, entity, evidence)
		prompt := fmt.Sprintf("What biological enhancement was documented in the %d soil study by %s on %s?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("No-till plots exhibited %d percent higher mycorrhizal fungal biomass", val1),
			"Intensive tillage produced superior subterranean microbial diversity",
			"Mycorrhizal fungi were completely destroyed by organic mulch applications",
			"Soil moisture levels showed no correlation with fungal root colonisation",
		}
		explanation := fmt.Sprintf("Sekuens DNA memastikan biomassa jamur mikoriza %d persen lebih tinggi pada lahan tanpa olah.", val1)
		tip := "Periksa perbandingan persentase biomassa jamur mikoriza pada tanah."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 7
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("bilingual subjects retained %d percent greater neural efficiency in executive control tasks", val1)
		contextStr := fmt.Sprintf("Cognitive scientists at %s published a neuroimaging study in %d tracking bilingual adults across visual stimulus tests. Functional MRI scans showed that %s relative to monolingual controls. The investigators linked these findings to enhanced neuroplastic cognitive reserve across ageing populations.", entity, year, evidence)
		prompt := fmt.Sprintf("In the %d neuroimaging study by %s on %s, what cognitive benefit was confirmed?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Bilingual participants exhibited %d percent greater neural efficiency in executive control", val1),
			"Monolingual adults performed significantly faster on sensory conflict tasks",
			"Bilingualism accelerated age-related neural atrophy in prefrontal networks",
			"Functional neuroimaging revealed identical cortical activation in all groups",
		}
		explanation := fmt.Sprintf("Pemindaian fMRI membuktikan efisiensi saraf %d persen lebih tinggi pada subjek bilingual.", val1)
		tip := "Hubungkan data efisiensi kontrol eksekutif dengan kemampuan bilingual."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 8
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("sulfur-oxidizing archaea supported an endemic biomass of %d kilograms per square meter", val1)
		contextStr := fmt.Sprintf("During deep-ocean submersible dives conducted in %d, marine biologists from %s explored volcanic abyssal vents. Quantitative sampling revealed that %s near mineral-rich thermal chimneys. This discovery highlights the remarkable density of chemosynthetic food webs in aphotic marine environments.", year, entity, evidence)
		prompt := fmt.Sprintf("According to the %d hydrothermal vent investigation by %s on %s, what biomass density was recorded?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Chemotrophic archaea sustained %d kilograms of biomass per square meter", val1),
			"Thermal plumes were sterile due to extreme mineral toxicity",
			"Benthic vent organisms relied entirely on surface organic matter precipitation",
			"Submersible sampling equipment malfunctioned before gathering density data",
		}
		explanation := fmt.Sprintf("Uji kuantitatif menunjukkan biomassa endemis mencapai %d kilogram per meter persegi.", val1)
		tip := "Cari angka kepadatan biomassa per meter persegi di sekitar cerobong hidrotermal."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
	// 9
	func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
		evidence := fmt.Sprintf("stratospheric aerosol residence time exceeded %d months", val1)
		contextStr := fmt.Sprintf("Atmospheric modelers at %s simulated volcanic sulfate dispersion in %d using high-altitude lidar arrays. Spectroscopic calibration proved that %s following high-magnitude explosive eruptions. The lingering particulate shield caused measurable temporary declines in hemispheric solar irradiance.", entity, year, evidence)
		prompt := fmt.Sprintf("In the %d volcanic particulate study by %s on %s, what aerosol duration was verified?", year, entity, topic)
		choices := []string{
			fmt.Sprintf("Sulfate aerosols persisted in the stratosphere for over %d months", val1),
			"Volcanic particulates settled completely to the ground within two weeks",
			"Aerosol dispersion produced an immediate collapse of global precipitation patterns",
			"Lidar detection systems failed to differentiate sulfate particles from cloud cover",
		}
		explanation := fmt.Sprintf("Kalibrasi lidar memastikan waktu tinggal aerosol di stratosfer melebihi %d bulan.", val1)
		tip := "Identifikasi durasi waktu tinggal partikel aerosol pada lapisan stratosfer."
		return contextStr, evidence, prompt, choices, explanation, tip
	},
}

var listeningScenarioBuilders = []listeningScenarioBuilder{
	// 0
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Receptionist: Good morning, %s information desk. How may I direct your call today?\nPatient: Hello, I would like to schedule an assessment regarding our %s initiative.\nReceptionist: Mr. Davies has an available slot on Thursday at %d:%02d in Consultation Room %d.\nPatient: Thursday suits me well. Is there an initial assessment charge due at the desk?\nReceptionist: Yes, the standard introductory consultation fee is %d pounds, payable on arrival.\nPatient: Thank you, I have marked Thursday at %d:%02d in Room %d in my diary.", place, topic, hour, minute, roomNum, fee, hour, minute, roomNum)
		prompt := fmt.Sprintf("According to the receptionist at %s regarding %s, what is the introductory consultation fee?", place, topic)
		choices := []string{
			fmt.Sprintf("%d pounds", fee),
			fmt.Sprintf("%d pounds with mandatory equipment levy", fee+25),
			"Free under local community health subsidy",
			"Fifty pounds flat rate for students",
		}
		explanation := fmt.Sprintf("Resepsionis mengonfirmasi bahwa biaya asesmen awal adalah %d pounds.", fee)
		return dialogue, prompt, choices, explanation
	},
	// 1
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Librarian: Helpdesk for %s, how can I assist you with %s today?\nStudent: Hi, I need to reserve a quiet study carrel for my thesis research.\nLibrarian: We have individual study carrel number %d available on the third floor starting at %d:%02d.\nStudent: Excellent. Does carrel %d provide dedicated Ethernet connections?\nLibrarian: Yes, and keycard access is valid until %d:00 every evening with a %d pounds deposit.\nStudent: Wonderful, please confirm my booking for carrel %d.", place, topic, roomNum, hour, minute, roomNum, hour+4, fee, roomNum)
		prompt := fmt.Sprintf("According to the librarian at %s regarding %s, which carrel number was reserved for the student?", place, topic)
		choices := []string{
			fmt.Sprintf("Carrel number %d on the third floor", roomNum),
			fmt.Sprintf("Carrel number %d in the basement archives", roomNum+10),
			"The open group study pavilion",
			"The digital multimedia cluster",
		}
		explanation := fmt.Sprintf("Pustakawan mengalokasikan ruang belajar individual nomor %d di lantai tiga.", roomNum)
		return dialogue, prompt, choices, explanation
	},
	// 2
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Tour Guide: Welcome aboard the excursion organized by %s. Please assemble by the gangway.\nPassenger: Excuse me, what time do we berth for our %s field session?\nTour Guide: We are scheduled to arrive at Pier %d at %d:%02d for our guided historical tour.\nPassenger: Will visitors have adequate time to tour the maritime dry docks on foot?\nTour Guide: Yes, passengers will have exactly %d minutes of guided walking time before departure.\nPassenger: Perfect, I will reassemble with the group at Pier %d.", place, topic, roomNum, hour, minute, fee, roomNum)
		prompt := fmt.Sprintf("According to the tour guide at %s regarding %s, how much walking time is allocated?", place, topic)
		choices := []string{
			fmt.Sprintf("%d minutes of guided walking time", fee),
			"Two full hours of unguided sightseeing",
			"Fifteen minutes for a rapid exterior photo stop",
			"Zero on-shore walking time is permitted",
		}
		explanation := fmt.Sprintf("Pemandu wisata menjelaskan bahwa waktu jalan kaki terpandu adalah %d menit.", fee)
		return dialogue, prompt, choices, explanation
	},
	// 3
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Adviser: %s office, good afternoon. Are you calling regarding semester accommodation in %s?\nApplicant: Yes, I would like to verify my assigned room in the student residences.\nAdviser: Your allocation is confirmed in Elm Court, Flat %d, with a monthly rent of %d pounds.\nApplicant: Does Flat %d include high-speed broadband and heating?\nAdviser: All utility bills and internet access are fully covered in the %d pounds monthly rate.\nApplicant: That is excellent. I will submit the security deposit for Flat %d.", place, topic, roomNum, fee*10, roomNum, fee*10, roomNum)
		prompt := fmt.Sprintf("In the consultation with %s regarding %s, what is the monthly rent for Flat %d in Elm Court?", place, topic, roomNum)
		choices := []string{
			fmt.Sprintf("%d pounds per month with all utilities included", fee*10),
			fmt.Sprintf("%d pounds excluding electricity and heating", fee*10-50),
			"Eight hundred pounds payable per term",
			"Free accommodation under exchange scholarship",
		}
		explanation := fmt.Sprintf("Petugas akomodasi menegaskan bahwa uang sewa adalah %d pounds per bulan.", fee*10)
		return dialogue, prompt, choices, explanation
	},
	// 4
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Coach: %s sports complex, how can I assist you with %s today?\nParent: Hello, I am looking to enrol my son in the youth training squad.\nCoach: Squad sessions take place every Wednesday at %d:%02d in Lane %d of the main pool.\nParent: How many children are coached in each instructional lane?\nCoach: We maintain a strict limit of %d participants per lane to ensure safety.\nParent: That is reassuring. We will be there on Wednesday at %d:%02d in Lane %d.", place, topic, hour, minute, roomNum%10+1, fee/5, hour, minute, roomNum%10+1)
		prompt := fmt.Sprintf("According to the head coach at %s regarding %s, what is the maximum number of swimmers per lane?", place, topic)
		choices := []string{
			fmt.Sprintf("Strictly limited to %d participants per lane", fee/5),
			"Up to twenty children without coach oversight",
			"Individual one-on-one sessions only",
			"Fifteen swimmers across two adjoining lanes",
		}
		explanation := fmt.Sprintf("Pelatih menjelaskan bahwa batas maksimal adalah %d peserta per lintasan.", fee/5)
		return dialogue, prompt, choices, explanation
	},
	// 5
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Clerk: Welcome to %s. How can I help with your journey on %s?\nCommuter: Hi, I need to book an express ticket for tomorrow afternoon.\nClerk: The express service departs from Platform %d at %d:%02d with an advance ticket fare of %d pounds.\nCommuter: Is seat reservation included in that advance ticket?\nClerk: Yes, a guaranteed seat in Carriage %d is included automatically with the %d pounds ticket.\nCommuter: Wonderful, please issue my advance ticket for Platform %d.", place, topic, roomNum%20+1, hour, minute, fee, roomNum%8+1, fee, roomNum%20+1)
		prompt := fmt.Sprintf("According to the booking clerk at %s regarding %s, what is the advance ticket fare?", place, topic)
		choices := []string{
			fmt.Sprintf("%d pounds", fee),
			fmt.Sprintf("%d pounds during peak commuter rush", fee+30),
			"Ninety-five pounds with weekend surcharge",
			"Complimentary with national rail pass",
		}
		explanation := fmt.Sprintf("Petugas stasiun menjelaskan bahwa harga tiket advance adalah %d pounds.", fee)
		return dialogue, prompt, choices, explanation
	},
	// 6
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Curator: Welcome to the gallery at %s. Are you inquiring about our %s exhibition?\nVisitor: Hello, are there audio tour guides available for the main gallery?\nCurator: Yes, digital audio transceivers can be hired at Kiosk %d for a fee of %d pounds.\nVisitor: How much time does the full exhibition tour take to listen to?\nCurator: The narrated audio commentary takes approximately %d minutes across four rooms.\nVisitor: Splendid, I will rent an audio handset from Kiosk %d.", place, topic, roomNum, fee/2, fee, roomNum)
		prompt := fmt.Sprintf("According to the gallery curator at %s regarding %s, what is the rental fee for an audio handset at Kiosk %d?", place, topic, roomNum)
		choices := []string{
			fmt.Sprintf("%d pounds", fee/2),
			fmt.Sprintf("%d pounds with exhibition catalog bundle", fee/2+15),
			"Free entry for all gallery patrons",
			"Thirty pounds security bond only",
		}
		explanation := fmt.Sprintf("Kurator galeri menyatakan bahwa biaya sewa audio guide adalah %d pounds.", fee/2)
		return dialogue, prompt, choices, explanation
	},
	// 7
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Inspector: %s regulatory division, how may I help with your %s project?\nVendor: Hello, I am finalizing our registration for the commercial exhibition.\nInspector: Your approved vendor stall is designated as Booth %d in Sector %d.\nVendor: Does Sector %d have an adequate commercial electrical supply?\nInspector: Yes, each vendor in Sector %d receives %d kilowatts of continuous circuit power.\nVendor: Perfect, I will display our compliance certificate at Booth %d.", place, topic, roomNum, roomNum/10+1, roomNum/10+1, roomNum/10+1, fee/4, roomNum)
		prompt := fmt.Sprintf("In the inspection dialogue with %s regarding %s, how much continuous power is supplied to Booth %d?", place, topic, roomNum)
		choices := []string{
			fmt.Sprintf("%d kilowatts of continuous circuit power", fee/4),
			fmt.Sprintf("%d kilowatts during emergency backup mode only", fee/4-2),
			"Zero electrical supply; vendors must bring diesel generators",
			"Twenty-five kilowatts on three-phase heavy commercial grids",
		}
		explanation := fmt.Sprintf("Inspektur menyatakan bahwa daya listrik kontinu yang disediakan adalah %d kilowatt.", fee/4)
		return dialogue, prompt, choices, explanation
	},
	// 8
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Agent: Equipment centre at %s, how can we outfit you for %s today?\nCyclist: Hi, I want to rent an all-terrain touring bicycle for the regional expedition.\nAgent: We have our premium trail bike available at a daily rate of %d pounds.\nCyclist: Does the rental package include a certified helmet and repair kit?\nAgent: Yes, safety gear, a helmet, and a lock are all included with the trail bike.\nCyclist: Excellent, I will take the trail bike for the full journey.", place, topic, fee)
		prompt := fmt.Sprintf("According to the equipment agent at %s regarding %s, what is the daily rental rate for the trail bike?", place, topic)
		choices := []string{
			fmt.Sprintf("%d pounds per day", fee),
			fmt.Sprintf("%d pounds per four-hour slot", fee-10),
			"Seventy pounds with compulsory insurance rider",
			"Free hire for local resident card holders",
		}
		explanation := fmt.Sprintf("Petugas rental mengonfirmasi harga sewa harian sepeda adalah %d pounds.", fee)
		return dialogue, prompt, choices, explanation
	},
	// 9
	func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
		dialogue := fmt.Sprintf("Registrar: Academic Assessment Office at %s, how can I assist you with %s?\nStudent: Hello, I am checking my assigned room for the oral language examination on Friday.\nRegistrar: Your speaking assessment is scheduled in Examination Room %d at %d:%02d.\nStudent: Do I need to present two forms of identity to the invigilator?\nRegistrar: Yes, your student identity badge and passport must be verified at Room %d.\nStudent: Understood, I will arrive at Room %d ten minutes prior to the start.", place, topic, roomNum, hour, minute, roomNum, roomNum)
		prompt := fmt.Sprintf("According to the assessment registrar at %s regarding %s, in which room will the exam take place?", place, topic)
		choices := []string{
			fmt.Sprintf("Examination Room %d", roomNum),
			fmt.Sprintf("Examination Room %d in the annex building", roomNum+10),
			"The main administrative auditorium",
			"The digital language laboratory",
		}
		explanation := fmt.Sprintf("Petugas registrasi mengonfirmasi bahwa ujian speaking diadakan di Ruang (Room) %d.", roomNum)
		return dialogue, prompt, choices, explanation
	},
}

func init() {
	domains := []struct {
		field, subject, metric, action, effect string
	}{
		{"Urban forestry", "canopy density", "transpiration cooling", "measured", "mitigated ambient microclimates"},
		{"Marine ecology", "kelp forest biomass", "carbon sequestration", "quantified", "enhanced localized buffering"},
		{"Renewable microgrids", "battery storage efficiency", "inverter dissipation", "calculated", "stabilized intermittent load demands"},
		{"Hydrological modeling", "groundwater recharge", "aquifer filtration", "documented", "curbed subterranean salinity intrusion"},
		{"Aeroelastic engineering", "wing flutter damping", "vibrational resonance", "evaluated", "improved aerodynamic fuel efficiency"},
		{"Volcanological monitoring", "sulfur dioxide emissions", "fumarole degassing", "recorded", "refined early eruption forecast models"},
		{"Agricultural genomics", "drought-tolerant maize", "photosynthetic yield", "demonstrated", "boosted crop resilience under arid stress"},
		{"Cognitive linguistics", "semantic retrieval speeds", "bilingual lexical access", "verified", "supported neuroplastic adaptation models"},
		{"Archaeological petrography", "ancient ceramic temper", "kiln firing temperatures", "identified", "mapped prehistoric trade networks"},
		{"Atmospheric chemistry", "tropospheric ozone flux", "photochemical oxidation", "assessed", "clarified urban smog formation kinetics"},
		{"Biomedical microfluidics", "capillary cell sorting", "separation fidelity", "established", "accelerated early diagnostic screening"},
		{"Paleoclimatology", "speleothem isotope ratios", "monsoon precipitation intensity", "reconstructed", "traced millennial climate cycles"},
		{"Industrial metallurgy", "titanium alloy grain size", "tensile fracture resistance", "tested", "prolonged aerospace airframe lifespans"},
		{"Acoustic ecology", "avian vocal pitch shifts", "traffic noise masking", "analyzed", "highlighted behavioural adaptation in songbirds"},
		{"Soil biogeochemistry", "humic acid binding", "heavy metal immobilization", "confirmed", "reduced phytotoxic runoff into waterways"},
		{"Architectural acoustics", "diffuser reflection coefficients", "reverberation decay times", "computed", "optimized speech clarity in auditoriums"},
		{"Benthic oceanography", "deep-sea sediment cores", "biogenic silica accumulation", "charted", "dated historical ocean circulation shifts"},
		{"Transportation planning", "bus rapid transit frequency", "modal shift percentages", "tabulated", "lowered peak commuter carbon footprints"},
		{"Plant physiology", "stomatal conductance rates", "evapotranspiration loss", "monitored", "improved irrigation scheduling protocols"},
		{"Computational genetics", "splice site prediction algorithms", "classification accuracy", "validated", "streamlined target gene annotation"},
	}

	methods := []string{
		"spectrographic sensors",
		"multispectral satellite arrays",
		"automated acoustic loggers",
		"submersible geochemical probes",
		"autonomous telemetry stations",
	}

	for len(readingStoryBuilders) < 100 {
		idx := len(readingStoryBuilders)
		d := domains[idx%len(domains)]
		method := methods[(idx/len(domains))%len(methods)]
		builder := func(year int, entity, topic string, itemCode, val1, val2 int) (string, string, string, []string, string, string) {
			evidence := fmt.Sprintf("sensor telemetry verified that %s achieved an index of %d benchmark units", d.metric, val1)
			contextStr := fmt.Sprintf("Field Study (%s): In %d, researchers from %s examined %s using %s during regional trials. Continuous %s across all monitoring transects. The empirical findings confirmed that systemic intervention %s across the surrounding biome.", d.field, year, entity, d.subject, method, evidence, d.effect)
			prompt := fmt.Sprintf("According to the %d investigation by %s on %s, what result was verified?", year, entity, d.field)
			choices := []string{
				fmt.Sprintf("Telemetry confirmed that %s reached an index of %d benchmark units", d.metric, val1),
				fmt.Sprintf("Baseline %s showed zero correlation with regional environmental parameters", d.subject),
				"Field measurement sensors failed to record valid metrics due to power interruption",
				"Researchers concluded that preliminary baseline benchmarks were fundamentally flawed",
			}
			explanation := fmt.Sprintf("Data telemetri sensor membuktikan bahwa %s mencapai indeks %d unit benchmark.", d.metric, val1)
			tip := fmt.Sprintf("Perhatikan angka spesifik untuk %s yang tercatat dalam teks.", d.metric)
			return contextStr, evidence, prompt, choices, explanation, tip
		}
		readingStoryBuilders = append(readingStoryBuilders, builder)
	}

	dialogueTopics := []struct {
		r1, r2, situation, item, loc string
	}{
		{"Registrar", "Student", "academic module registration", "module credits", "Lecture Theatre"},
		{"Coordinator", "Volunteer", "community clean-up orientation", "safety gear packages", "Assembly Point"},
		{"Officer", "Resident", "council recycling container delivery", "standard recycling bins", "Depot Yard"},
		{"Curator", "Artist", "sculpture exhibition load-in", "gallery pedestals", "Main Wing"},
		{"Technician", "Researcher", "microscope calibration appointment", "laser lenses", "Optics Lab"},
		{"Manager", "Applicant", "internship onboarding briefing", "access keycards", "Conference Suite"},
		{"Inspector", "Contractor", "building safety pre-audit", "fire safety baffles", "North Block"},
		{"Trainer", "Participant", "first aid certification workshop", "resuscitation mannequins", "Training Hall"},
		{"Host", "Guest", "eco-lodge cottage check-in", "welcome hampers", "Pine Cabin"},
		{"Dispatcher", "Driver", "courier parcel collection route", "priority crates", "Loading Bay"},
		{"Agent", "Tenant", "residential lease agreement review", "deposit receipts", "Branch Office"},
		{"Supervisor", "Mechanic", "fleet vehicle diagnostics check", "hydraulic filters", "Service Bay"},
		{"Steward", "Hiker", "national park wilderness permit", "trail maps", "Ranger Post"},
		{"Clerk", "Tourist", "botanical garden guided shuttle", "express passes", "Visitor Pavilion"},
		{"Secretary", "Author", "literary festival panel schedule", "moderator notes", "Green Room"},
		{"Doctor", "Patient", "telehealth follow-up session", "prescription forms", "Consultation Unit"},
		{"Director", "Volunteer", "animal shelter adoption intake", "kennel blankets", "Adoption Foyer"},
		{"Admin", "Member", "fitness centre induction booking", "locker tokens", "Studio B"},
		{"Guide", "Traveler", "castle fortress heritage tour", "audio transceivers", "Gatehouse"},
		{"Representative", "Client", "solar battery storage quote", "warranty certificates", "Design Hub"},
	}

	channels := []string{"inquiry desk", "advisory counter", "liaison desk", "central registry", "coordination service"}

	for len(listeningScenarioBuilders) < 100 {
		idx := len(listeningScenarioBuilders)
		dt := dialogueTopics[idx%len(dialogueTopics)]
		channel := channels[(idx/len(dialogueTopics))%len(channels)]
		builder := func(itemCode int, place, topic string, roomNum, hour, minute, fee int) (string, string, []string, string) {
			dialogue := fmt.Sprintf("%s: Good day from %s, %s here. Are you checking on %s for %s?\n%s: Yes, I am following up on the scheduled appointment.\n%s: Let me retrieve your file. The booking is confirmed for %s %d at %d:%02d.\n%s: Does that booking allocate the required %s?\n%s: Exactly, %d units of %s are officially allocated with a confirmation fee of %d pounds.\n%s: Wonderful, I will report to %s %d at %d:%02d to sign the documentation.",
				dt.r1, place, channel, dt.situation, topic,
				dt.r2,
				dt.r1, dt.loc, roomNum, hour, minute,
				dt.r2, dt.item,
				dt.r1, fee/2, dt.item, fee,
				dt.r2, dt.loc, roomNum, hour, minute)
			prompt := fmt.Sprintf("According to the %s at %s regarding %s, how many units of %s are allocated?", dt.r1, place, dt.situation, dt.item)
			choices := []string{
				fmt.Sprintf("%d units of %s", fee/2, dt.item),
				fmt.Sprintf("%d units delivered without official confirmation", fee/2+5),
				"Zero units due to unexpected supply chain shortages",
				"Fifty units reserved exclusively for emergency contingency",
			}
			explanation := fmt.Sprintf("Pembicara menegaskan bahwa alokasi resmi adalah sebanyak %d unit %s.", fee/2, dt.item)
			return dialogue, prompt, choices, explanation
		}
		listeningScenarioBuilders = append(listeningScenarioBuilders, builder)
	}
}
