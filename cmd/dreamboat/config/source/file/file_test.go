package file

import (
	"log"
	"testing"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/structs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSource_Load(t *testing.T) {
	t.Skip("test is flaky (@lukasz)")

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

			if err := s.Load(ab, true); tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, c, tt.wantC)
		})
	}
}

type TestChange struct {
}

func (tc *TestChange) OnConfigChange(change structs.OldNew) error {
	log.Println("change structs.OldNew", change)
	return nil
}
