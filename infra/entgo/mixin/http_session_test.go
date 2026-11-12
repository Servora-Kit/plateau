package mixin

import (
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
)

func TestHTTPSessionMixinSchemaContract(t *testing.T) {
	var m HTTPSessionMixin

	fields := m.Fields()
	if len(fields) != 3 {
		t.Fatalf("len(Fields()) = %d, want 3", len(fields))
	}

	idDesc := fields[0].Descriptor()
	if idDesc.Name != "id" || idDesc.StorageKey != "token" || !idDesc.Sensitive || idDesc.SchemaType[dialect.Postgres] != "text" {
		t.Fatalf("unexpected id descriptor: %+v", idDesc)
	}

	dataDesc := fields[1].Descriptor()
	if dataDesc.Name != "data" || !dataDesc.Sensitive {
		t.Fatalf("unexpected data descriptor: %+v", dataDesc)
	}

	expiryDesc := fields[2].Descriptor()
	if expiryDesc.Name != "expiry" || expiryDesc.SchemaType[dialect.Postgres] != "timestamptz" {
		t.Fatalf("unexpected expiry descriptor: %+v", expiryDesc)
	}

	indexes := m.Indexes()
	if len(indexes) != 1 {
		t.Fatalf("len(Indexes()) = %d, want 1", len(indexes))
	}
	idxDesc := indexes[0].Descriptor()
	if len(idxDesc.Fields) != 1 || idxDesc.Fields[0] != "expiry" {
		t.Fatalf("unexpected index fields: %+v", idxDesc.Fields)
	}

	annotations := m.Annotations()
	if len(annotations) != 1 {
		t.Fatalf("len(Annotations()) = %d, want 1", len(annotations))
	}
	sqlAnn, ok := annotations[0].(entsql.Annotation)
	if !ok || sqlAnn.Table != "sessions" {
		t.Fatalf("unexpected annotation: %#v", annotations[0])
	}
}
