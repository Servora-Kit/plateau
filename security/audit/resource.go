package audit

import (
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type fieldRedactor interface {
	RedactFields()
}

// RedactClone returns a deep clone of msg with all protoc-gen-redact sensitive
// fields cleared recursively across nested messages, lists, and maps without
// mutating msg.
func RedactClone(msg proto.Message) proto.Message {
	if msg == nil {
		return nil
	}
	clone := proto.Clone(msg)
	redactRecursive(clone.ProtoReflect())
	return clone
}

func redactRecursive(m protoreflect.Message) {
	if !m.IsValid() {
		return
	}
	if r, ok := m.Interface().(fieldRedactor); ok {
		r.RedactFields()
	}

	m.Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		if fd.IsList() {
			if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
				list := val.List()
				for i := range list.Len() {
					redactRecursive(list.Get(i).Message())
				}
			}
			return true
		}
		if fd.IsMap() {
			if fd.MapValue().Kind() == protoreflect.MessageKind || fd.MapValue().Kind() == protoreflect.GroupKind {
				val.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
					redactRecursive(v.Message())
					return true
				})
			}
			return true
		}
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			redactRecursive(val.Message())
		}
		return true
	})
}

// ExtractResource inspects AIP google.api.resource and google.api.resource_reference
// annotations on msg (and its direct singular message fields) to resolve the
// canonical resource type (e.g. "iam.plateau.dev/User") and resource name
// (e.g. "users/019...").
func ExtractResource(msg proto.Message) (resourceType, resourceName string) {
	if msg == nil {
		return "", ""
	}
	m := msg.ProtoReflect()
	if !m.IsValid() {
		return "", ""
	}

	// 1. Message itself declares google.api.resource.
	if resType, resName, ok := extractMessageResource(m); ok {
		return resType, resName
	}

	fields := m.Descriptor().Fields()

	// 2. Direct string field annotated with google.api.resource_reference.
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.Kind() != protoreflect.StringKind || fd.IsList() || fd.IsMap() || fd.Options() == nil {
			continue
		}
		ref, _ := proto.GetExtension(fd.Options(), annotations.E_ResourceReference).(*annotations.ResourceReference)
		if ref == nil {
			continue
		}
		refType := ref.GetType()
		if refType == "" {
			refType = ref.GetChildType()
		}
		var val string
		if m.Has(fd) {
			val = m.Get(fd).String()
		}
		if val != "" || resourceType == "" {
			resourceType = refType
			resourceName = val
			if val != "" {
				return resourceType, resourceName
			}
		}
	}

	// 3. Direct singular message field whose message type declares google.api.resource
	// (e.g. CreateUserRequest.user, UpdateUserRequest.user, DisableUserResponse.user).
	for i := range fields.Len() {
		fd := fields.Get(i)
		if (fd.Kind() != protoreflect.MessageKind && fd.Kind() != protoreflect.GroupKind) || fd.IsList() || fd.IsMap() {
			continue
		}
		subOpts := fd.Message().Options()
		if subOpts == nil {
			continue
		}
		subDesc, _ := proto.GetExtension(subOpts, annotations.E_Resource).(*annotations.ResourceDescriptor)
		if subDesc == nil {
			continue
		}
		subType := subDesc.GetType()
		var subName string
		if m.Has(fd) {
			_, subName, _ = extractMessageResource(m.Get(fd).Message())
		}
		if subName != "" {
			return subType, subName
		}
		if resourceType == "" {
			resourceType = subType
		}
	}

	return resourceType, resourceName
}

func extractMessageResource(m protoreflect.Message) (resourceType, resourceName string, ok bool) {
	md := m.Descriptor()
	if md == nil || md.Options() == nil {
		return "", "", false
	}
	desc, _ := proto.GetExtension(md.Options(), annotations.E_Resource).(*annotations.ResourceDescriptor)
	if desc == nil {
		return "", "", false
	}
	resourceType = desc.GetType()

	fields := md.Fields()
	var nameField protoreflect.FieldDescriptor
	if configured := desc.GetNameField(); configured != "" {
		if fd := fields.ByName(protoreflect.Name(configured)); fd != nil && fd.Kind() == protoreflect.StringKind {
			nameField = fd
		}
	}
	if nameField == nil {
		for i := range fields.Len() {
			fd := fields.Get(i)
			if fd.Kind() != protoreflect.StringKind || fd.IsList() || fd.IsMap() || fd.Options() == nil {
				continue
			}
			behaviors, _ := proto.GetExtension(fd.Options(), annotations.E_FieldBehavior).([]annotations.FieldBehavior)
			for _, b := range behaviors {
				if b == annotations.FieldBehavior_IDENTIFIER {
					nameField = fd
					break
				}
			}
			if nameField != nil {
				break
			}
		}
	}
	if nameField == nil {
		if fd := fields.ByName("name"); fd != nil && fd.Kind() == protoreflect.StringKind && !fd.IsList() && !fd.IsMap() {
			nameField = fd
		}
	}
	if nameField != nil && m.IsValid() && m.Has(nameField) {
		resourceName = m.Get(nameField).String()
	}
	return resourceType, resourceName, true
}
