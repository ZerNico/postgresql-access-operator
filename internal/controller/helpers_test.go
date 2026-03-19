/*
Copyright 2026.

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

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
)

func TestRequeueResult_DefaultInterval(t *testing.T) {
	tmpl := dbv1alpha1.SQLTemplate{}
	result := requeueResult(tmpl)

	if result.RequeueAfter < defaultRequeueInterval {
		t.Errorf("RequeueAfter = %v, want >= %v", result.RequeueAfter, defaultRequeueInterval)
	}
	if result.RequeueAfter > defaultRequeueInterval+maxRequeueJitter {
		t.Errorf("RequeueAfter = %v, want <= %v", result.RequeueAfter, defaultRequeueInterval+maxRequeueJitter)
	}
}

func TestRequeueResult_CustomInterval(t *testing.T) {
	custom := 2 * time.Minute
	tmpl := dbv1alpha1.SQLTemplate{
		RequeueInterval: &metav1.Duration{Duration: custom},
	}
	result := requeueResult(tmpl)

	if result.RequeueAfter < custom {
		t.Errorf("RequeueAfter = %v, want >= %v", result.RequeueAfter, custom)
	}
	if result.RequeueAfter > custom+maxRequeueJitter {
		t.Errorf("RequeueAfter = %v, want <= %v", result.RequeueAfter, custom+maxRequeueJitter)
	}
}

func TestRetryResult_DefaultInterval(t *testing.T) {
	tmpl := dbv1alpha1.SQLTemplate{}
	result := retryResult(tmpl)

	if result.RequeueAfter < defaultRetryInterval {
		t.Errorf("RequeueAfter = %v, want >= %v", result.RequeueAfter, defaultRetryInterval)
	}
	if result.RequeueAfter > defaultRetryInterval+maxRequeueJitter {
		t.Errorf("RequeueAfter = %v, want <= %v", result.RequeueAfter, defaultRetryInterval+maxRequeueJitter)
	}
}

func TestRetryResult_CustomInterval(t *testing.T) {
	custom := 15 * time.Second
	tmpl := dbv1alpha1.SQLTemplate{
		RetryInterval: &metav1.Duration{Duration: custom},
	}
	result := retryResult(tmpl)

	if result.RequeueAfter < custom {
		t.Errorf("RequeueAfter = %v, want >= %v", result.RequeueAfter, custom)
	}
	if result.RequeueAfter > custom+maxRequeueJitter {
		t.Errorf("RequeueAfter = %v, want <= %v", result.RequeueAfter, custom+maxRequeueJitter)
	}
}

func TestAddJitter(t *testing.T) {
	base := 30 * time.Second

	// Run multiple times to verify jitter is bounded
	for range 100 {
		result := addJitter(base)
		if result < base {
			t.Errorf("addJitter(%v) = %v, want >= %v", base, result, base)
		}
		if result > base+maxRequeueJitter {
			t.Errorf("addJitter(%v) = %v, want <= %v", base, result, base+maxRequeueJitter)
		}
	}
}
