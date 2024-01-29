/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package wait

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"

	armadav1 "opendev.org/airship/armada-operator/api/v1"
)

type StatusType string

type Status struct {
	StatusType
	Msg string
}

const (
	Ready   StatusType = "READY"
	Skipped StatusType = "SKIPPED"
	Unready StatusType = "UNREADY"
	Error   StatusType = "ERROR"
)

// WaitOptions phase run command
type WaitOptions struct {
	Getter        cache.Getter
	Namespace     string
	LabelSelector string
	ResourceType  string
	Timeout       time.Duration
	Logger        logr.Logger
}

func getObjectStatus(obj interface{}) Status {
	switch v := obj.(type) {
	case *armadav1.ArmadaChart:
		return isArmadaChartReady(v)
	default:
		return Status{Error, fmt.Sprintf("Unable to cast an object to any type %s\n", obj)}
	}
}

func allMatch(logger logr.Logger, store cache.Store, obj runtime.Object) (bool, error) {
	for _, item := range store.List() {
		if obj != nil && item == obj {
			continue
		}
		status := getObjectStatus(item)
		logger.Info(fmt.Sprintf("all match object %T is ready returned %s\n", item, status.StatusType))
		logger.Info(status.Msg)
		if status.StatusType != Ready && status.StatusType != Skipped {
			logger.Info(fmt.Sprintf("all match exiting false due to %s\n", status.StatusType))
			return false, nil
		}
	}
	logger.Info("all objects are ready\n")
	return true, nil
}

func processEvent(logger logr.Logger, event watch.Event) (StatusType, error) {
	metaObj, err := meta.Accessor(event.Object)
	if err != nil {
		return Error, err
	}

	logger.Info(fmt.Sprintf("watch event: type=%s, name=%s, namespace=%s, resource_ver %s",
		event.Type, metaObj.GetName(), metaObj.GetNamespace(), metaObj.GetResourceVersion()))

	if event.Type == "ERROR" {
		return Error, errors.New(fmt.Sprintf("resource %s: got error event %s", metaObj.GetName(), event.Object))
	}

	status := getObjectStatus(event.Object)
	logger.Info(fmt.Sprintf("object type: %T, status: %s", event.Object, status.Msg))
	return status.StatusType, nil
}

func isArmadaChartReady(ac *armadav1.ArmadaChart) Status {
	if ac.Status.ObservedGeneration == ac.Generation {
		for _, cond := range ac.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				return Status{Ready, fmt.Sprintf("armadachart %s ready", ac.GetName())}
			}
		}
	}
	return Status{Unready, fmt.Sprintf("Waiting for armadachart %s to be ready", ac.GetName())}
}

// Wait runs the phase
func (c *WaitOptions) Wait(parent context.Context) error {
	c.Logger.Info(fmt.Sprintf("armada-go wait , namespace %s labels %s type %s timeout %s", c.Namespace, c.LabelSelector, c.ResourceType, c.Timeout))

	ctx, cancelFunc := watchtools.ContextWithOptionalTimeout(parent, c.Timeout)
	defer cancelFunc()

	lw := cache.NewFilteredListWatchFromClient(c.Getter, "armadacharts", c.Namespace, func(options *metav1.ListOptions) {
		options.LabelSelector = c.LabelSelector
		c.Logger.Info(fmt.Sprintf("Label selector applied %s", options))
	})

	var cacheStore cache.Store

	cpu := func(store cache.Store) (bool, error) {
		cacheStore = store
		if len(store.List()) == 0 {
			c.Logger.Info(fmt.Sprintf("skipping non-required wait, no resources found.\n"))
			return true, nil
		}
		return allMatch(c.Logger, cacheStore, nil)
	}

	cfu := func(event watch.Event) (bool, error) {
		if ready, err := processEvent(c.Logger, event); ready != Ready || err != nil {
			return false, err
		}

		return allMatch(c.Logger, cacheStore, event.Object)
	}

	_, err := watchtools.UntilWithSync(ctx, lw, nil, cpu, cfu)
	c.Logger.Info(fmt.Sprintf("wait completed %s\n", c.LabelSelector))
	return err
}
