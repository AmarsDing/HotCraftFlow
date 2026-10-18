//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{
		Features: []gen.Feature{
			gen.FeatureEntQL,
			gen.FeatureModifier,
			gen.FeaturePrivacy,
			// IC UpsertWithEvidenceTx 依赖 Create.OnConflict（B-6 dedup_key 幂等键，
			// PostgreSQL INSERT ... ON CONFLICT DO UPDATE）。
			gen.FeatureUpsert,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
