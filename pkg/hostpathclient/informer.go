package hostpathclient

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// NewIndexerInformer is a copy of the cache.NewIndexerInformer function, but allows custom process function
func NewIndexerInformer(lw cache.ListerWatcher, objType runtime.Object, h cache.ResourceEventHandler, indexers cache.Indexers, builder ProcessorBuilder) (cache.Indexer, cache.Controller) {
	indexer := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, indexers)

	cfg := &cache.Config{
		Queue:            cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{KeyFunction: cache.MetaNamespaceKeyFunc, KnownObjects: indexer}),
		ListerWatcher:    lw,
		ObjectType:       objType,
		FullResyncPeriod: defaultResyncPeriod,
		RetryOnError:     false,
		Process:          builder(indexer, h),
	}
	return indexer, cache.New(cfg)
}

// DefaultProcessor is based on the Process function from cache.NewIndexerInformer except it does a conversion.
func DefaultProcessor(convert ToFunc) ProcessorBuilder {
	return func(indexer cache.Indexer, handler cache.ResourceEventHandler) cache.ProcessFunc {
		return func(obj interface{}) error {
			for _, d := range obj.(cache.Deltas) {
				switch d.Type {
				case cache.Sync, cache.Added, cache.Updated:
					obj, err := convert(d.Object)
					if err != nil {
						return err
					}
					if old, exists, err := indexer.Get(obj); err == nil && exists {
						if err := indexer.Update(obj); err != nil {
							return err
						}
						handler.OnUpdate(old, obj)
					} else {
						if err := indexer.Add(obj); err != nil {
							return err
						}
						handler.OnAdd(obj)
					}
				case cache.Deleted:
					var obj interface{}
					obj, ok := d.Object.(cache.DeletedFinalStateUnknown)
					if !ok {
						var err error
						obj, err = convert(d.Object)
						if err != nil {
							return err
						}
					}

					if err := indexer.Delete(obj); err != nil {
						return err
					}
					handler.OnDelete(obj)
				}
			}
			return nil
		}
	}
}

const defaultResyncPeriod = 0
