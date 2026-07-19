package database

func DefaultCriterionRubric(id string) ([]string, []CriterionRubricBand, bool) {
	switch id {
	case "crt-uniform":
		return []string{
				"All Scouts: shirt, BA badge sewn on, necker and ID badge.",
				"Lead should be taken from the Patrol Leader.",
				"Scottish Scouts: kilt, black shoes, lovat green socks with scout green flashes and scout belt.",
				"International Scouts: look for consistency and use the Patrol Leader as the standard when in doubt.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Entire patrol dressed correctly.", "BA badge sewn on.", "Smart appearance."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Most scouts correctly dressed.", "Minor issues only, such as a missing belt or untidy necker."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Several scouts not correct.", "Dress is inconsistent across the patrol."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Many scouts have incomplete uniform."}},
				{Label: "1-2", Title: "Not attempted", MinValue: 1, MaxValue: 2, Bullets: []string{"No obvious attempt to get the patrol into uniform."}},
			}, true
	case "crt-inside-tents":
		return []string{
				"Interiors should be tidy and clean.",
				"Personal kit should be in bags and bedding rolled up.",
				"Ground sheet should be folded back to form a passage in patrol tents.",
				"The store tent should be tidy, clean and usable for food prep in bad weather.",
				"Food storage should be clearly separated from camp kit.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Interiors are very tidy.", "Kit is stored neatly and presented uniformly.", "Bedding rolled and passage clear.", "Store tent is tidy and ready for use."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Generally tidy.", "Only minor clutter present."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Mixed organisation.", "Some items are not stored correctly."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Tents are untidy.", "Mess and litter visible in tents."}},
				{Label: "1-2", Title: "Very poor", MinValue: 1, MaxValue: 2, Bullets: []string{"No attempt to prepare or tidy inside tents."}},
			}, true
	case "crt-tent-structure":
		return []string{
				"Walls hung appropriately; if rain is forecast they may be kept down.",
				"Tent should be pitched correctly.",
				"Storm guys and side guys should be tight and aligned.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Guy rope lines are tight and straight.", "Correct peg sizes used in each area of the tent."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Good pitching overall.", "Only minor rope adjustments required."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Functional but uneven lines.", "Some loose ropes or sagging structure."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Barely functional.", "Contents could get wet if it rains."}},
				{Label: "1-2", Title: "Unsafe", MinValue: 1, MaxValue: 2, Bullets: []string{"Unsafe and needs repitching."}},
			}, true
	case "crt-food-hygiene":
		return []string{
				"Food should be raised off the ground.",
				"Utensils and cooking equipment should be clean, dry and stored separately from food.",
				"Meat and dairy products must be returned to subcamp staff after every meal.",
				"Dry goods should be in watertight containers with no excessive food buildup.",
				"Use one dish towel per meal and keep a dirty towel basket or box.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"All food stored correctly.", "Cooking equipment is very clean and ready for use.", "Food prep surfaces are clean."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Cooking equipment is clean.", "Food storage is adequate."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Food scraps visible.", "Equipment clean but perhaps wet.", "Dirty dish towels are being reused."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Meat or dairy not returned.", "Poor hygiene or storage.", "Multiple food waste issues visible.", "Cooking equipment is not clean."}},
				{Label: "1-2", Title: "Unsafe", MinValue: 1, MaxValue: 2, Bullets: []string{"Equipment stored on the ground.", "Immediate intervention required for patrol wellbeing."}},
			}, true
	case "crt-structure":
		return []string{
				"Main structure should be stable, tightly lashed and fit for purpose.",
				"Table should be clean with plastic covering in place.",
				"Cover should be secure.",
				"Fire should be stable, level and safely insulated with earth.",
				"Ashes should be cleared unless still hot.",
				"Two red fire buckets should be at least three quarters full with clean water and no debris.",
				"Wash-up gadgets should be present and operational.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Lashings are tight.", "Fire pit is clean with ample turf soil.", "Wash area is clean and sturdy.", "Table is clean and cover secure.", "Decorations and notices are tasteful and safe.", "Useful wash-up gadgets are in place."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Solid operational structure.", "Only minor discrepancies in fire pit, cover or wash-up area.", "Minor lashing work required."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Structure is functional but needs improvement to lashings or operational areas.", "Ashes left on the fire."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Fire buckets inadequate or contain foreign objects.", "Weak structure.", "Fire requires more turf for safety.", "Lashings need attention.", "Table cover is loose."}},
				{Label: "1-2", Title: "Unsafe", MinValue: 1, MaxValue: 2, Bullets: []string{"Immediate intervention required to prevent collapse or unsafe fire area."}},
			}, true
	case "crt-chopping-area":
		return []string{
				"Wood should be chopped and ready for the next meal.",
				"Chippings should stay on the mat and the wood should be covered.",
				"Chopping area should be tidy.",
				"Tools should be stored safely, preferably in the store tent.",
				"Boundary should be clearly marked.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Wood is graded and enough for the next meal.", "Chopping area is tidy.", "Tools stored safely.", "Boundary markings are adequate.", "Wood would stay dry if it rains."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Good chopping and storage process.", "Well organised, with only visible chippings or minor boundary issues.", "Plenty of wood, though grading could improve."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Enough wood, but poorly graded.", "Boundary fence needs work.", "Wood risks getting wet if it rains."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Tools left unguarded or unsafe.", "Not enough wood for the next meal.", "No attempt at grading.", "No proper rain protection for wood."}},
				{Label: "1-2", Title: "Unsafe", MinValue: 1, MaxValue: 2, Bullets: []string{"No chopping area marked.", "People could easily stray into the chopping area.", "No wood available.", "Tools left open to the weather.", "Area not suitable for the patrol to run a fire safely."}},
			}, true
	case "crt-general-area":
		return []string{
				"Gateway should show the patrol name, be functional and safe.",
				"Boundary fence should be fit for purpose, tight and not used as a clothes line.",
				"Camp gadgets should be fit for purpose.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Gateway is safe, well decorated and clear.", "Boundary fencing is clear.", "Useful gadgets are present and in use."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Gateway is safe but could be improved.", "Only minor boundary improvements required.", "An attempt has been made at gadgets."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Gateway is functional but minimal.", "No real attempt at gadgets.", "Boundary fence needs tidying."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Gateway or boundary exists but is not safe.", "Intervention needed that day."}},
				{Label: "1-2", Title: "Missing", MinValue: 1, MaxValue: 2, Bullets: []string{"No gateway or boundary in place."}},
			}, true
	case "crt-personal-hygiene":
		return []string{
				"Nails and hands should be clean with evidence of washing.",
				"Teeth should be clean with evidence of brushing.",
				"Overall personal appearance should show that scouts are washing and looking after themselves.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Hands, nails, faces and teeth are clean.", "The whole patrol clearly looks after themselves."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Mostly clean, with only minor issues affecting appearance."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Some of the patrol lack basic hygiene, such as dirty hands or unwashed hair.", "Washing materials do not seem to be used consistently."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Poor standards across the patrol.", "Improvements are required that day."}},
				{Label: "1-2", Title: "Immediate action", MinValue: 1, MaxValue: 2, Bullets: []string{"Patrol should not continue morning activities until personal hygiene is addressed."}},
			}, true
	case "crt-team-work":
		return []string{
				"Scottish and overseas Patrol Leaders should communicate and get along.",
				"Scottish patrol should work together with the Patrol Leader leading by example.",
				"Overseas patrol should work together with the Patrol Leader leading by example.",
				"The full patrol should communicate well and work together to get tasks done.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Excellent teamwork.", "Patrol Leader leadership is evident.", "Scottish and overseas patrols are integrated."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Good teamwork evident.", "Only mixed engagement between some members of the joint patrol."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Some evidence that the patrol is not working together consistently for the benefit of the patrol area."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Poor cooperation between patrol members.", "Frequent interventions required from the subcamp team.", "No evidence of a rota to share roles."}},
				{Label: "1-2", Title: "Patrol in collapse", MinValue: 1, MaxValue: 2, Bullets: []string{"Immediate intervention from leaders required to keep the patrol functioning."}},
			}, true
	case "crt-litter":
		return []string{
				"There should be no litter or food scraps on or in front of the site.",
				"The area behind sleeping tents should be tidy.",
				"Use fresh rubbish bags for inspection.",
				"Recycling should be sorted into the correct waste streams.",
				"Food waste and paper should be burnt where appropriate.",
				"Slop bucket should be emptied daily, with a clean cover and tidy surrounding area.",
			}, []CriterionRubricBand{
				{Label: "9-10", Title: "Excellent", MinValue: 9, MaxValue: 10, Bullets: []string{"Completely litter free.", "Recycling is organised.", "Slop bucket is empty and clean."}},
				{Label: "7-8", Title: "Good", MinValue: 7, MaxValue: 8, Bullets: []string{"Only very minor litter, difficult to find."}},
				{Label: "5-6", Title: "Fair", MinValue: 5, MaxValue: 6, Bullets: []string{"Multiple instances of litter visible from a distance.", "Slop bucket still contains scraps.", "Recycling is mixed with general waste."}},
				{Label: "3-4", Title: "Poor", MinValue: 3, MaxValue: 4, Bullets: []string{"Slop bucket not emptied.", "Rubbish bags are full or not in place."}},
				{Label: "1-2", Title: "Severe issue", MinValue: 1, MaxValue: 2, Bullets: []string{"Campsite resembles a festival site on departure day.", "Serious improvements are required before activity sessions."}},
			}, true
	default:
		return nil, nil, false
	}
}
