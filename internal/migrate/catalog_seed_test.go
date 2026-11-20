package migrate

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Static integrity checks of 036_equipment_catalog.sql against plans/gear-bag/02-technical.md §2.1
// (no database needed). Full DB verification is documented in test/test.md (EQUIP-04).

func readCatalogSQL(t *testing.T) string {
	t.Helper()
	b, err := sqlFS.ReadFile("sql/036_equipment_catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func section(t *testing.T, sql, from, to string) string {
	t.Helper()
	i := strings.Index(sql, from)
	j := strings.Index(sql, to)
	if i < 0 || j < 0 || j <= i {
		t.Fatalf("section %q..%q not found", from, to)
	}
	return sql[i:j]
}

// brand -> categories (sport/code) exactly as listed in the §2.1 brand table.
var specBrandCategories = map[string]string{
	"Babolat":     "TENNIS/RACKET TENNIS/STRING TENNIS/SHOES TENNIS/OVERGRIP PADEL/PALA PADEL/OVERGRIP",
	"Wilson":      "TENNIS/RACKET TENNIS/STRING TENNIS/OVERGRIP PADEL/OVERGRIP",
	"Head":        "TENNIS/RACKET TENNIS/STRING PADEL/PALA SQUASH/RACKET SQUASH/STRING SQUASH/SHOES TENNIS/OVERGRIP",
	"Yonex":       "TENNIS/RACKET TENNIS/OVERGRIP",
	"Tecnifibre":  "TENNIS/RACKET TENNIS/STRING SQUASH/RACKET SQUASH/STRING",
	"Dunlop":      "TENNIS/RACKET SQUASH/RACKET",
	"Prince":      "TENNIS/RACKET SQUASH/RACKET",
	"Luxilon":     "TENNIS/STRING",
	"Solinco":     "TENNIS/STRING",
	"ASICS":       "TENNIS/SHOES PADEL/SHOES SQUASH/SHOES PICKLEBALL/SHOES",
	"Nike":        "TENNIS/SHOES PICKLEBALL/SHOES",
	"Adidas":      "TENNIS/SHOES PADEL/PALA PICKLEBALL/SHOES",
	"New Balance": "TENNIS/SHOES",
	"Bullpadel":   "PADEL/PALA PADEL/SHOES PADEL/OVERGRIP",
	"Nox":         "PADEL/PALA PADEL/SHOES PADEL/OVERGRIP",
	"Siux":        "PADEL/PALA",
	"Varlion":     "PADEL/PALA",
	"Joma":        "PADEL/SHOES",
	"Karakal":     "SQUASH/RACKET",
	"Salming":     "SQUASH/SHOES",
	"Ashaway":     "SQUASH/STRING",
	"Butterfly":   "PING_PONG/BLADE PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH",
	"Stiga":       "PING_PONG/BLADE PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH",
	"Donic":       "PING_PONG/BLADE PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH",
	"Yasaka":      "PING_PONG/BLADE",
	"DHS":         "PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH",
	"Tibhar":      "PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH",
	"Joola":       "PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH PICKLEBALL/PADDLE PICKLEBALL/OVERGRIP",
	"Franklin":    "PICKLEBALL/PADDLE",
	"Gamma":       "PICKLEBALL/PADDLE PICKLEBALL/OVERGRIP",
	"Paddletek":   "PICKLEBALL/PADDLE",
	"CRBN":        "PICKLEBALL/PADDLE",
	"Six Zero":    "PICKLEBALL/PADDLE",
	"Selkirk":     "PICKLEBALL/PADDLE PICKLEBALL/OVERGRIP",
}

func TestCatalogSeed_BrandsComplete(t *testing.T) {
	sql := readCatalogSQL(t)
	body := section(t, sql, "-- Seed: brands", "-- Seed: models")
	re := regexp.MustCompile(`\('([^']+)', '(#[0-9A-Fa-f]{6})'\)`)
	got := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		if _, dup := got[m[1]]; dup {
			t.Fatalf("duplicate brand %s", m[1])
		}
		got[m[1]] = m[2]
	}
	if len(got) != 34 || len(specBrandCategories) != 34 {
		t.Fatalf("brands=%d spec=%d want 34", len(got), len(specBrandCategories))
	}
	for name := range specBrandCategories {
		if _, ok := got[name]; !ok {
			t.Errorf("brand %q missing from seed", name)
		}
	}
	for name, hex := range map[string]string{"Babolat": "#0B3D91", "Joola": "#FFD100", "Six Zero": "#FF5A00", "CRBN": "#000000"} {
		if got[name] != hex {
			t.Errorf("brand %s accent=%s want %s", name, got[name], hex)
		}
	}
}

func TestCatalogSeed_CategoriesComplete(t *testing.T) {
	sql := readCatalogSQL(t)
	body := section(t, sql, "-- Seed: categories", "-- Seed: brands")
	re := regexp.MustCompile(`\('([A-Z_]+)',\s+'([A-Z_]+)',`)
	got := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		got[m[1]+"/"+m[2]] = true
	}
	want := strings.Fields(`TENNIS/RACKET TENNIS/STRING TENNIS/SHOES TENNIS/OVERGRIP PADEL/PALA PADEL/SHOES PADEL/OVERGRIP
		SQUASH/RACKET SQUASH/STRING SQUASH/SHOES PING_PONG/BLADE PING_PONG/RUBBER_FH PING_PONG/RUBBER_BH
		PICKLEBALL/PADDLE PICKLEBALL/SHOES PICKLEBALL/OVERGRIP`)
	if len(want) != 16 || len(got) != 16 {
		t.Fatalf("categories got=%d want=%d", len(got), len(want))
	}
	for _, k := range want {
		if !got[k] {
			t.Errorf("category %s missing", k)
		}
	}
	// Squash/STRING is HOURS only (AC-2 table authoritative); Tennis/STRING is HOURS_OR_DAYS 15h/90d.
	if !regexp.MustCompile(`'SQUASH',\s+'STRING',\s+'RACKET', 2, TRUE, TRUE, 'HOURS',\s+15, NULL`).MatchString(body) {
		t.Error("Squash/STRING must be HOURS 15 with no days")
	}
	if !regexp.MustCompile(`'TENNIS',\s+'STRING',\s+'RACKET', 2, TRUE, TRUE, 'HOURS_OR_DAYS', 15, 90`).MatchString(body) {
		t.Error("Tennis/STRING must be HOURS_OR_DAYS 15/90")
	}
}

func TestCatalogSeed_ModelsCoverEveryBrandCategoryPair(t *testing.T) {
	sql := readCatalogSQL(t)
	body := section(t, sql, "-- Seed: models", "-- equipment: EXTEND")
	re := regexp.MustCompile(`\('([A-Z_]+)', '([A-Z_]+)', '([^']+)', '([^']+)'\)`)
	perPair := map[string][]string{} // sport/code|brand -> models
	seen := map[string]bool{}
	total := 0
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		key := m[1] + "/" + m[2] + "|" + m[3] + "|" + m[4]
		if seen[key] {
			t.Fatalf("duplicate model row %s", key)
		}
		seen[key] = true
		perPair[m[1]+"/"+m[2]+"|"+m[3]] = append(perPair[m[1]+"/"+m[2]+"|"+m[3]], m[4])
		total++
	}
	if total != 167 {
		t.Fatalf("models=%d want 167", total)
	}

	brandCats := map[string][]string{}
	for pair, models := range perPair {
		parts := strings.SplitN(pair, "|", 2)
		if len(models) < 2 || len(models) > 4 {
			t.Errorf("%s has %d models, want 2-4", pair, len(models))
		}
		brandCats[parts[1]] = append(brandCats[parts[1]], parts[0])
	}
	for brand, cats := range specBrandCategories {
		want := strings.Fields(cats)
		got := brandCats[brand]
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, " ") != strings.Join(got, " ") {
			t.Errorf("brand %s categories\n got  %v\n want %v", brand, got, want)
		}
	}

	// Spot-check models from the §2.1 model table (incl. long names / punctuation).
	for _, k := range []string{
		"TENNIS/RACKET|Prince|Textreme Tour 100", "TENNIS/STRING|Tecnifibre|X-One Biphase",
		"PADEL/PALA|Varlion|Lethal Pro", "PADEL/SHOES|Joma|T.Ace", "SQUASH/RACKET|Head|Graphene Touch Radical",
		"PING_PONG/BLADE|Yasaka|Ma Lin Extra Offensive", "PING_PONG/RUBBER_FH|Donic|Bluestorm Z1 Turbo",
		"PING_PONG/RUBBER_BH|Butterfly|Dignics 09C", "PICKLEBALL/PADDLE|Franklin|Ben Johns Falcon",
		"PICKLEBALL/PADDLE|Paddletek|Tempest Wave", "PICKLEBALL/OVERGRIP|Gamma|EZ Wrap",
	} {
		if !seen[k] {
			t.Errorf("model %s missing", k)
		}
	}
}

func TestCatalogSeed_RubberFHandBHAreIdentical(t *testing.T) {
	sql := readCatalogSQL(t)
	body := section(t, sql, "-- Seed: models", "-- equipment: EXTEND")
	fh := regexp.MustCompile(`'PING_PONG', 'RUBBER_FH', '([^']+)', '([^']+)'`).FindAllStringSubmatch(body, -1)
	bh := regexp.MustCompile(`'PING_PONG', 'RUBBER_BH', '([^']+)', '([^']+)'`).FindAllStringSubmatch(body, -1)
	if len(fh) != 13 || len(fh) != len(bh) {
		t.Fatalf("fh=%d bh=%d want 13/13", len(fh), len(bh))
	}
	for i := range fh {
		if fh[i][1] != bh[i][1] || fh[i][2] != bh[i][2] {
			t.Fatalf("row %d differs fh=%v bh=%v", i, fh[i], bh[i])
		}
	}
}

func TestCatalogMigration_KeepsRollbackSafetyAndBackfill(t *testing.T) {
	sql := readCatalogSQL(t)
	if strings.Contains(sql, "DROP INDEX") {
		t.Error("migration must not drop/alter the legacy uq_equipment_active index")
	}
	for _, must := range []string{
		"e.type = 'STRING'", "e.type = 'SHOES'", "ALTER COLUMN lineage_id SET NOT NULL",
		"uq_equipment_active_category", "CREATE TABLE IF NOT EXISTS match_equipment_usage",
		"equipment_set_lineage",
	} {
		if !strings.Contains(sql, must) {
			t.Errorf("migration missing %q", must)
		}
	}
	// Backfill must run after the category seed.
	if strings.Index(sql, "AC-22 backfill (categories") < strings.Index(sql, "-- Seed: categories") {
		t.Error("backfill must come after category seed")
	}
}
