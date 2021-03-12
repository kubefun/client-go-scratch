package store

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const resyncPeriod = time.Second * 180

type DynamicCache struct {
	ctx    context.Context
	cancel context.CancelFunc

	informerFactory dynamicinformer.DynamicSharedInformerFactory
	knownInformers  sync.Map
	unwatched       sync.Map

	removeChan chan schema.GroupVersionResource
}

type interuptibleInformer struct {
	stopCh   chan struct{}
	informer informers.GenericInformer
	gvr      schema.GroupVersionResource
}

func NewDynamicCache(ctx context.Context, client dynamic.Interface, namespace string) (*DynamicCache, error) {
	ctx, cancel := context.WithCancel(ctx)

	dc := &DynamicCache{
		ctx:             ctx,
		cancel:          cancel,
		informerFactory: dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, resyncPeriod, namespace, nil),
		knownInformers:  sync.Map{},
		unwatched:       sync.Map{},
		removeChan:      make(chan schema.GroupVersionResource),
	}

	go dc.worker()

	return dc, nil
}

func (d *DynamicCache) worker() {
	for {
		select {
		case <-d.ctx.Done():
			d.knownInformers.Range(func(k, v interface{}) bool {
				ii := v.(interuptibleInformer)
				close(ii.stopCh)
				return true
			})
			return
		case gvr := <-d.removeChan:
			d.unwatched.Store(gvr, true)
			v, ok := d.knownInformers.LoadAndDelete(gvr)
			if ok {
				ii := v.(interuptibleInformer)
				close(ii.stopCh)
			}
		case <-time.After(time.Second * 1):
			continue
		}
	}
}

func (d *DynamicCache) isUnwatched(gvr schema.GroupVersionResource) bool {
	_, ok := d.unwatched.Load(gvr)
	return ok
}

func (d *DynamicCache) forResource(gvr schema.GroupVersionResource) interuptibleInformer {
	v, ok := d.knownInformers.Load(gvr)
	if !ok {
		i := d.informerFactory.ForResource(gvr)
		stopCh := make(chan struct{})
		i.Informer().SetWatchErrorHandler(d.watchErrorHandler(gvr, stopCh))
		go i.Informer().Run(stopCh)
		ii := interuptibleInformer{
			stopCh,
			i,
			gvr,
		}
		d.knownInformers.Store(gvr, ii)
		return ii
	}
	ii := v.(interuptibleInformer)
	return ii
}

func (d *DynamicCache) WaitForCacheSync() bool {
	var ret bool
	d.knownInformers.Range(func(k, v interface{}) bool {
		ii := v.(interuptibleInformer)
		ret = cache.WaitForCacheSync(ii.stopCh, ii.informer.Informer().HasSynced)
		return ret
	})
	return ret
}

func (d *DynamicCache) Watch(gvr schema.GroupVersionResource) error {
	_, err := d.ListerForResource(gvr)
	return err
}

func (d *DynamicCache) ListerForResource(gvr schema.GroupVersionResource) (cache.GenericLister, error) {
	if d.isUnwatched(gvr) {
		return nil, fmt.Errorf("unable to get Lister for %s, watcher was unable to start", gvr)
	}

	ii := d.forResource(gvr)
	return ii.informer.Lister(), nil
}

func (d *DynamicCache) watchErrorHandler(gvr schema.GroupVersionResource, stopCh chan struct{}) func(*cache.Reflector, error) {
	return func(r *cache.Reflector, err error) {
		logger := log.Default()
		logger.Println("error starting watcher ", err.Error())
		d.removeChan <- gvr
	}
}
