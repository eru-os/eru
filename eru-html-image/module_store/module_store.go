package module_store

import (
	"github.com/eru-os/eru/eru-store/store"
)

type StoreHolder struct {
	Store ModuleStoreI
}

type ModuleStoreI interface {
	store.StoreI
}

type ModuleStore struct {
	store.FileStore
}
