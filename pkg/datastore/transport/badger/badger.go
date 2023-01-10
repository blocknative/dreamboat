package badger

import (
	badg "github.com/ipfs/go-ds-badger2"
)

func Open(datadir string) (*badg.Datastore, error) {
	storage, err := badg.NewDatastore(datadir, &badg.DefaultOptions)
	if err != nil {
		//logger.WithError(err).Error("failed to initialize datastore")
		return nil, err
	}

	return storage, err

}
