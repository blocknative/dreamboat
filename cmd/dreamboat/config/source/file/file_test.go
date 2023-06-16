package file

import (
	"encoding/json"
	"log"
	"reflect"
	"testing"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/structs"
)

func TestSource_Load(t *testing.T) {
	type fields struct {
		filepath string
	}
	tests := []struct {
		name    string
		fields  fields
		wantC   config.Config
		wantErr bool
	}{
		{
			name: "happy",
			fields: fields{
				filepath: "./parse_test.ini",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSource(tt.fields.filepath)
			//c := &config.Config{}
			c := config.DefaultConfig()
			ab := &c
			tc := &TestChange{}
			ab.Api.SubscribeForUpdates(tc)
			err := s.Load(ab, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("Source.Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			a, _ := json.Marshal(c)
			t.Log("a", string(a))
			if !reflect.DeepEqual(&c, tt.wantC) {
				t.Errorf("Source.Load() = %v, want %v", c, tt.wantC)
			}
		})
	}
}

type TestChange struct {
}

func (tc *TestChange) OnConfigChange(change structs.OldNew) error {
	log.Println("change structs.OldNew", change)
	return nil
}
