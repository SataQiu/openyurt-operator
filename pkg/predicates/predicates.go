/*
Copyright 2021 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package predicates

import (
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ResourceWithNamespaceName returns a predicate that returns true only if the provided resource with given namespace and name.
func ResourceWithNamespaceName(logger logr.Logger, namespace, name string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfNamespaceNameMatch(logger.WithValues("predicate", "updateEvent"), e.ObjectNew, namespace, name)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfNamespaceNameMatch(logger.WithValues("predicate", "createEvent"), e.Object, namespace, name)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfNamespaceNameMatch(logger.WithValues("predicate", "deleteEvent"), e.Object, namespace, name)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfNamespaceNameMatch(logger.WithValues("predicate", "genericEvent"), e.Object, namespace, name)
		},
	}
}

func processIfNamespaceNameMatch(logger logr.Logger, obj client.Object, namespace, name string) bool {
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	if namespace == obj.GetNamespace() && name == obj.GetName() {
		log.V(4).Info("Resource matches name, will attempt to map resource")
		return true
	}
	log.V(4).Info("Resource does not match name, will not attempt to map resource")
	return false
}

// ResourceLabelChanged returns a predicate that returns true only if the target label value was changed or the object was created or deleted
func ResourceLabelChanged(logger logr.Logger, labelKey string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfLabelChanged(logger.WithValues("predicate", "updateEvent"), e.ObjectOld, e.ObjectNew, labelKey)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func processIfLabelChanged(logger logr.Logger, old client.Object, new client.Object, name string) bool {
	// return early if no name was set.
	if name == "" {
		return false
	}

	kind := strings.ToLower(new.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithValues("namespace", new.GetNamespace(), kind, new.GetName())
	oldLabels := old.GetLabels()
	newLabels := new.GetLabels()
	if oldLabels[name] != newLabels[name] {
		log.V(4).Info("Resource label was changed, will attempt to map resource")
		return true
	}
	log.V(4).Info("Resource label was not changed, will not attempt to map resource")
	return false
}
