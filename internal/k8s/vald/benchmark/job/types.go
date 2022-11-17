//
// Copyright (C) 2019-2022 vdaas.org vald team <vald@vdaas.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package job

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BenchmarkJobSpec struct {
	Target     *BenchmarkTarget
	Dataset    *BenchmarkDataset
	Replica    int
	Repetition int
	JobType    string
	Dimension  int
	Epsilon    float32
	Radius     float32
	Iter       int
	Num        int32
	MinNum     int32
	Timeout    string
	Rules      []*BenchmarkJobRule
}

type BenchmarkJobStatus string

const (
	BenchmarkJobNotReady  = BenchmarkJobStatus("NotReady")
	BenchmarkJobAvailable = BenchmarkJobStatus("Available")
	BenchmarkJobHealthy   = BenchmarkJobStatus("Healthy")
)

type BenchmarkTarget struct {
	Host string
	Port int
}

type BenchmarkDataset struct {
	Name    string
	Group   string
	Indexes int
	Range   *BenchmarkDatasetRange
}

type BenchmarkDatasetRange struct {
	Start int
	End   int
}

type BenchmarkJobRule struct {
	Name string
	Type string
}

type BenchmarkJob struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Spec   BenchmarkJobSpec
	Status BenchmarkJobStatus
}

type BenchmarkJobList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []BenchmarkJob
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkDataset) DeepCopyInto(out *BenchmarkDataset) {
	*out = *in
	if in.Range != nil {
		in, out := &in.Range, &out.Range
		*out = new(BenchmarkDatasetRange)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkDataset.
func (in *BenchmarkDataset) DeepCopy() *BenchmarkDataset {
	if in == nil {
		return nil
	}
	out := new(BenchmarkDataset)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkDatasetRange) DeepCopyInto(out *BenchmarkDatasetRange) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkDatasetRange.
func (in *BenchmarkDatasetRange) DeepCopy() *BenchmarkDatasetRange {
	if in == nil {
		return nil
	}
	out := new(BenchmarkDatasetRange)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkJobRule) DeepCopyInto(out *BenchmarkJobRule) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkJobRule.
func (in *BenchmarkJobRule) DeepCopy() *BenchmarkJobRule {
	if in == nil {
		return nil
	}
	out := new(BenchmarkJobRule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkJobSpec) DeepCopyInto(out *BenchmarkJobSpec) {
	*out = *in
	if in.Target != nil {
		in, out := &in.Target, &out.Target
		*out = new(BenchmarkTarget)
		**out = **in
	}
	if in.Dataset != nil {
		in, out := &in.Dataset, &out.Dataset
		*out = new(BenchmarkDataset)
		(*in).DeepCopyInto(*out)
	}
	if in.Rules != nil {
		in, out := &in.Rules, &out.Rules
		*out = make([]*BenchmarkJobRule, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(BenchmarkJobRule)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkJobSpec.
func (in *BenchmarkJobSpec) DeepCopy() *BenchmarkJobSpec {
	if in == nil {
		return nil
	}
	out := new(BenchmarkJobSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkTarget) DeepCopyInto(out *BenchmarkTarget) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkTarget.
func (in *BenchmarkTarget) DeepCopy() *BenchmarkTarget {
	if in == nil {
		return nil
	}
	out := new(BenchmarkTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkJob) DeepCopyInto(out *BenchmarkJob) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkOperator.
func (in *BenchmarkJob) DeepCopy() *BenchmarkJob {
	if in == nil {
		return nil
	}
	out := new(BenchmarkJob)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BenchmarkJob) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BenchmarkJobList) DeepCopyInto(out *BenchmarkJobList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BenchmarkJob, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BenchmarkOperatorList.
func (in *BenchmarkJobList) DeepCopy() *BenchmarkJobList {
	if in == nil {
		return nil
	}
	out := new(BenchmarkJobList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BenchmarkJobList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

/*
spec:
	name: xxx
	# Job実行のrule {immediately | after | exclue | time | concurrent}
	rules:
		- type: string # exclude
			name: string # insert-zzz
		- type: string # after
			name: string # yyy
	target:
		host: string
		port: string
	dataset:
		name: string # dataset_name
		group: string # e.g., { train | test } (ANN benchmarkの場合)
									# (hdf5に準拠した内容)
		indexes: number # dataset_range
		range:
			start: number
			end: number
  jobType: string # insert, search, upsert, etc
	timeout: xxx
	rps: number
*/
