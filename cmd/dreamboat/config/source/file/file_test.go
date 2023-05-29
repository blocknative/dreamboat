package file

import (
	"reflect"
	"testing"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
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
			s := &Source{
				filepath: tt.fields.filepath,
			}
			gotC, err := s.Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Source.Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotC, tt.wantC) {
				t.Errorf("Source.Load() = %v, want %v", gotC, tt.wantC)
			}
		})
	}
}
