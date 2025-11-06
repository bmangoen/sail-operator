// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manifestcustomization

import (
	"path/filepath"

	xv1alpha1 "github.com/istio-ecosystem/sail-operator/api/x/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceMatcher provides functionality to match Kubernetes resources against ResourceMatcher criteria
type ResourceMatcher struct {
	matchers []xv1alpha1.ResourceMatcher
}

// NewResourceMatcher creates a new ResourceMatcher with the given matchers
func NewResourceMatcher(matchers []xv1alpha1.ResourceMatcher) *ResourceMatcher {
	return &ResourceMatcher{
		matchers: matchers,
	}
}

// Matches returns true if the resource matches ANY of the configured matchers
func (m *ResourceMatcher) Matches(resource *unstructured.Unstructured) bool {
	for _, matcher := range m.matchers {
		if m.matchesSingle(resource, matcher) {
			return true
		}
	}
	return false
}

// matchesSingle checks if a resource matches a single matcher
func (m *ResourceMatcher) matchesSingle(resource *unstructured.Unstructured, matcher xv1alpha1.ResourceMatcher) bool {
	// Match Group
	if matcher.Group != "" && resource.GetObjectKind().GroupVersionKind().Group != matcher.Group {
		return false
	}

	// Match Kind (required)
	if resource.GetKind() != matcher.Kind {
		return false
	}

	// Match Namespace (with wildcard support)
	if matcher.Namespace != "" {
		if !matchesPattern(resource.GetNamespace(), matcher.Namespace) {
			return false
		}
	}

	// Match Name (with wildcard support)
	if matcher.Name != "" {
		if !matchesPattern(resource.GetName(), matcher.Name) {
			return false
		}
	}

	return true
}

// matchesPattern checks if a string matches a pattern with wildcard support
func matchesPattern(value, pattern string) bool {
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		// If the pattern is invalid, fall back to exact match
		return value == pattern
	}
	return matched
}
