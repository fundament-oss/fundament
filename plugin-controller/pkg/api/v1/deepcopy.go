package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *PluginInstallation) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *PluginInstallation) DeepCopy() *PluginInstallation {
	if in == nil {
		return nil
	}
	out := new(PluginInstallation)
	in.DeepCopyInto(out)
	return out
}

func (in *PluginInstallation) DeepCopyInto(out *PluginInstallation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *PluginInstallationSpec) DeepCopyInto(out *PluginInstallationSpec) {
	*out = *in
	if in.ClusterRoles != nil {
		out.ClusterRoles = make([]string, len(in.ClusterRoles))
		copy(out.ClusterRoles, in.ClusterRoles)
	}
	if in.Config != nil {
		out.Config = make(map[string]string, len(in.Config))
		for k, v := range in.Config {
			out.Config[k] = v
		}
	}
}

func (in *PluginInstallationList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *PluginInstallationList) DeepCopy() *PluginInstallationList {
	if in == nil {
		return nil
	}
	out := new(PluginInstallationList)
	in.DeepCopyInto(out)
	return out
}

func (in *PluginInstallationList) DeepCopyInto(out *PluginInstallationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]PluginInstallation, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
