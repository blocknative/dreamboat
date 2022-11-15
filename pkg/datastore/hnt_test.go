package datastore_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	datastore "github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/stretchr/testify/require"
)

func TestHNTs_Serialize(t *testing.T) {

	headrOne := randomHeaderAndTrace()
	headrTwo := randomHeaderAndTrace()

	headrTwo.Trace.BuilderPubkey = headrOne.Trace.BuilderPubkey

	type fields struct {
		Input []structs.HR
	}
	tests := []struct {
		name   string
		fields fields
		wantB  []byte
	}{
		{
			name: "test1",
			fields: fields{
				Input: []structs.HR{
					{
						HeaderAndTrace: randomHeaderAndTrace(),
						Slot:           12345,
					},
					{
						HeaderAndTrace: randomHeaderAndTrace(),
						Slot:           12345,
					},
					{
						HeaderAndTrace: headrOne,
						Slot:           12345,
					},
					{ // should be skipped
						HeaderAndTrace: headrTwo,
						Slot:           12345,
					},
					{ // shoudl be skipped
						HeaderAndTrace: headrOne,
						Slot:           12345,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := datastore.NewHNTs()

			for i, inp := range tt.fields.Input {
				tt.fields.Input[i].Marshaled, _ = json.Marshal(inp.HeaderAndTrace)

				h.Add(tt.fields.Input[i])
			}

			normal := []structs.HeaderAndTrace{}
			maxPerformance := []structs.HeaderAndTrace{}
			for _, v := range tt.fields.Input[:4] {
				normal = append(normal, v.HeaderAndTrace)
			}

			for _, v := range tt.fields.Input[:3] {
				maxPerformance = append(maxPerformance, v.HeaderAndTrace)
			}

			sort.Slice(maxPerformance, func(i, j int) bool {
				return maxPerformance[i].Trace.Value.Cmp(&maxPerformance[j].Trace.Value) > 0
			})

			gotB := h.Serialize()
			a := []structs.HeaderAndTrace{}
			err := json.Unmarshal(gotB, &a)

			require.NoError(t, err)
			require.Len(t, a, 4)

			gotC := h.SerializeMaxProfit()
			b := []structs.HeaderAndTrace{}
			err = json.Unmarshal(gotC, &b)

			require.NoError(t, err)
			require.Len(t, b, 3)

			if reflect.DeepEqual(normal[0], normal[1]) ||
				reflect.DeepEqual(maxPerformance[0], maxPerformance[1]) ||
				reflect.DeepEqual(a[0], a[1]) ||
				reflect.DeepEqual(b[0], b[1]) {
				t.Errorf("Copy error")
			}

			if !reflect.DeepEqual(normal, a) {
				t.Errorf("HNTs.Serialize() = %v, want %v", gotB, tt.wantB)
			}

			if !reflect.DeepEqual(maxPerformance, b) {
				t.Errorf("HNTs.SerializeMaxProfit() = %v, want %v", gotC, tt.wantB)
			}

		})
	}
}
