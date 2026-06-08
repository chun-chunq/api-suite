package sanctions

import (
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Vladimir Putin", "vladimir putin"},
		{"VTB-Bank", "vtb bank"},
		{"  ALPHABANK  ", "alphabank"},
	}
	for _, c := range cases {
		got := normalizeName(c.in)
		if got != c.want {
			t.Errorf("normalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseXML_Empty(t *testing.T) {
	xml := `<export generationDate="2024-01-01"></export>`
	entities, err := parseXML([]byte(xml))
	if err != nil {
		t.Fatalf("parseXML error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(entities))
	}
}

func TestParseXML_SinglePerson(t *testing.T) {
	xml := `<export generationDate="2024-01-15">
  <sanctionEntity logicalId="ent-001">
    <subjectType classificationCode="P"/>
    <nameAlias firstName="Vladimir" lastName="Putin" wholeName="Vladimir Putin"/>
    <nameAlias wholeName="Владимир Путин"/>
    <regulation programme="RUSSIA" numberTitle="Council Regulation (EU) No 269/2014"/>
  </sanctionEntity>
</export>`
	entities, err := parseXML([]byte(xml))
	if err != nil {
		t.Fatalf("parseXML error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	e := entities[0]
	if e.ID != "ent-001" {
		t.Errorf("ID = %q, want ent-001", e.ID)
	}
	if e.SubjectType != "person" {
		t.Errorf("SubjectType = %q, want person", e.SubjectType)
	}
	if len(e.Names) != 2 {
		t.Errorf("expected 2 names, got %d", len(e.Names))
	}
	if e.Programme != "RUSSIA" {
		t.Errorf("Programme = %q, want RUSSIA", e.Programme)
	}
}

func TestParseXML_Entity(t *testing.T) {
	xml := `<export generationDate="2024-01-15">
  <sanctionEntity logicalId="ent-002">
    <subjectType classificationCode="E"/>
    <nameAlias wholeName="VTB Bank"/>
    <address city="Moscow" countryDescription="Russia"/>
    <regulation programme="RUSSIA" numberTitle="Regulation (EU) 269/2014"/>
  </sanctionEntity>
</export>`
	entities, err := parseXML([]byte(xml))
	if err != nil {
		t.Fatalf("parseXML error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	e := entities[0]
	if e.SubjectType != "entity" {
		t.Errorf("SubjectType = %q, want entity", e.SubjectType)
	}
	if len(e.Addresses) == 0 || !strings.Contains(e.Addresses[0], "Moscow") {
		t.Errorf("expected Moscow address, got %v", e.Addresses)
	}
}

func TestIndexSearchInMemory(t *testing.T) {
	// Build a minimal index directly (no HTTP download)
	idx := &Index{
		nameIdx: make(map[string][]int),
	}
	idx.entities = []Entity{
		{ID: "1", SubjectType: "person", Names: []Name{{WholeName: "Vladimir Putin"}}, Programme: "RUSSIA"},
		{ID: "2", SubjectType: "entity", Names: []Name{{WholeName: "VTB Bank"}}, Programme: "RUSSIA"},
		{ID: "3", SubjectType: "entity", Names: []Name{{WholeName: "Gazprom"}}, Programme: "RUSSIA"},
	}
	// Build name index
	for i, e := range idx.entities {
		for _, n := range e.Names {
			key := normalizeName(n.WholeName)
			idx.nameIdx[key] = append(idx.nameIdx[key], i)
			for _, tok := range strings.Fields(key) {
				idx.nameIdx[tok] = appendUnique(idx.nameIdx[tok], i)
			}
		}
	}
	idx.loaded.Store(true)

	results := idx.Search("Putin", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'Putin', got %d", len(results))
	}
	results = idx.Search("vtb", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'vtb', got %d", len(results))
	}

	matches := idx.Check("Gazprom")
	if len(matches) == 0 {
		t.Error("expected Gazprom to be found")
	}
	matches = idx.Check("Deutsche Bank")
	if len(matches) != 0 {
		t.Errorf("expected Deutsche Bank to not be found, got %v", matches)
	}
}

func TestMapSubjectType(t *testing.T) {
	if mapSubjectType("P") != "person" {
		t.Error("P should map to person")
	}
	if mapSubjectType("E") != "entity" {
		t.Error("E should map to entity")
	}
	if mapSubjectType("S") != "ship" {
		t.Error("S should map to ship")
	}
}
