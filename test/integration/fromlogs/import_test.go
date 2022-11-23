package fromlogs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseCSV(t *testing.T) {

	tests := []struct {
		name string
	}{
		{name: "sanity check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := openFile("./test.csv")
			defer o.Close()

			require.NoError(t, err)

			r, err := parseCSV(o)
			require.NoError(t, err)
			require.Len(t, r.Bids, 77)
			require.Len(t, r.BuilderBlockStored, 115)
		})
	}
}
