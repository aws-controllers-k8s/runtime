// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package annotation

import (
	"errors"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

const (
	LateInitializationAttempt = "services.k8s.aws/late-initialization-attempt"
)

// GetNumLateInitializationAttempt returns the number of late initialization
// attempts made. This value is stored in the k8s resource annotation
// 'services.k8s.aws/late-initialization-attempt'
func GetNumLateInitializationAttempt(metav1Obj metav1.Object) int {
	// Default is 1
	if ackcompare.IsNil(metav1Obj) || metav1Obj.GetAnnotations() == nil {
		return 1
	}
	if numLateInitAttemptStr, ok := metav1Obj.GetAnnotations()[LateInitializationAttempt]; ok {
		numLateInitAttempt, err := strconv.Atoi(numLateInitAttemptStr)
		if err == nil {
			return numLateInitAttempt
		}
	}
	return 1
}

// IncrementNumLateInitializationAttempt reads the number of late initialization
// attempts from k8s resource annotation, increments by 1, and stores back into
// k8s resource annotation
// This method updates the object the passed in the parameter inplace. Make sure
// to pass a pointer object for updates to reflect after method call.
// NOTE: AWSResource.MetaObject() method for all ACK resources returns a pointer
// , so this method is safe to use without return type.
// Also See https://github.com/aws-controllers-k8s/community/issues/905
func IncrementNumLateInitializationAttempt(metav1Obj metav1.Object) error {
	if ackcompare.IsNil(metav1Obj) {
		return errors.New("metav1Obj parameter should not be nil")
	}
	annotations := metav1Obj.GetAnnotations()
	if annotations != nil {
		if currentNumAttemptStr, ok := annotations[LateInitializationAttempt]; ok {
			if currentNumAttempt, err := strconv.Atoi(currentNumAttemptStr); err == nil {
				return SetNumLateInitializationAttempt(metav1Obj, currentNumAttempt+1)
			}
		}
	}
	// If late initialization attempt annotation is not present or value is not
	// a number, reset to 1
	return SetNumLateInitializationAttempt(metav1Obj, 1)
}

// SetNumLateInitializationAttempt stores 'numAttempt' into k8s resource annotation
// 'services.k8s.aws/late-initialization-attempt'
// This method updates the object the passed in the parameter inplace. Make sure
// to pass a pointer object for updates to reflect after method call.
// NOTE: AWSResource.MetaObject() method for all ACK resources returns a pointer
// , so this method is safe to use without return type.
// Also See: https://github.com/aws-controllers-k8s/community/issues/905
func SetNumLateInitializationAttempt(metav1Obj metav1.Object, numAttempt int) error {
	if ackcompare.IsNil(metav1Obj) {
		return errors.New("metav1Obj parameter should not be nil")
	}
	annotations := make(map[string]string)
	if metav1Obj.GetAnnotations() != nil {
		annotations = metav1Obj.GetAnnotations()
	}
	annotations[LateInitializationAttempt] = strconv.Itoa(numAttempt)
	metav1Obj.SetAnnotations(annotations)
	return nil
}

// RemoveLateInitializationAttempt removes the 'services.k8s.aws/late-initialization-attempt'
// annotation from k8s resource annotations
// This method updates the object the passed in the parameter inplace. Make sure
// to pass a pointer object for updates to reflect after method call.
// NOTE: AWSResource.MetaObject() method for all ACK resources returns a pointer
// , so this method is safe to use without return type.
// Also See https://github.com/aws-controllers-k8s/community/issues/905
func RemoveLateInitializationAttempt(metav1Obj metav1.Object) {
	if ackcompare.IsNotNil(metav1Obj) && metav1Obj.GetAnnotations() != nil {
		annotations := metav1Obj.GetAnnotations()
		delete(annotations, LateInitializationAttempt)
		metav1Obj.SetAnnotations(annotations)
	}
}
