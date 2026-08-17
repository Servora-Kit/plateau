package main

import (
	"strings"
	"testing"

	authzpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authz/v1"
	"github.com/Servora-Kit/plateau/internal/codegen/plugintest"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type methodSpec struct {
	name string
	rule *authzpb.AuthzRule
}

type serviceSpec struct {
	name           string
	serviceDefault *authzpb.AuthzRule
	methods        []methodSpec
}

type fileSpec struct {
	name     string
	protoPkg string
	goPkg    string
	generate bool
	services []serviceSpec
}

func runPluginScenario(t *testing.T, files []fileSpec) (*protogen.Plugin, error) {
	t.Helper()
	request := &pluginpb.CodeGeneratorRequest{
		ProtoFile: plugintest.DescriptorClosure(authzpb.File_plateau_security_authz_v1_annotations_proto),
		Parameter: proto.String("paths=source_relative"),
	}
	for _, file := range files {
		request.ProtoFile = append(request.ProtoFile, authzFileDescriptor(file))
		if file.generate {
			request.FileToGenerate = append(request.FileToGenerate, file.name)
		}
	}
	plugin, err := protogen.Options{}.New(request)
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	return plugin, generate(plugin)
}

func authzFileDescriptor(file fileSpec) *descriptorpb.FileDescriptorProto {
	descriptor := &descriptorpb.FileDescriptorProto{
		Name:       proto.String(file.name),
		Package:    proto.String(file.protoPkg),
		Syntax:     proto.String(protoreflect.Proto3.String()),
		Dependency: []string{"google/protobuf/descriptor.proto", "plateau/security/authz/v1/annotations.proto"},
		Options:    &descriptorpb.FileOptions{GoPackage: proto.String(file.goPkg)},
	}
	descriptor.MessageType = append(descriptor.MessageType,
		&descriptorpb.DescriptorProto{
			Name:      proto.String("Request"),
			OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("choice_target")}},
			NestedType: []*descriptorpb.DescriptorProto{{
				Name:    proto.String("LabelsEntry"),
				Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("key"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
					{Name: proto.String("value"), Number: proto.Int32(2), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				},
			}},
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("id"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				{Name: proto.String("nested"), Number: proto.Int32(2), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String("." + file.protoPkg + ".Nested")},
				{Name: proto.String("tags"), Number: proto.Int32(3), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				{Name: proto.String("number"), Number: proto.Int32(4), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()},
				{Name: proto.String("enabled"), Number: proto.Int32(5), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
				{Name: proto.String("payload"), Number: proto.Int32(6), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()},
				{Name: proto.String("ratio"), Number: proto.Int32(7), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()},
				{Name: proto.String("kind"), Number: proto.Int32(8), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String("." + file.protoPkg + ".Kind")},
				{Name: proto.String("choice"), Number: proto.Int32(9), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), OneofIndex: proto.Int32(0)},
				{Name: proto.String("labels"), Number: proto.Int32(10), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String("." + file.protoPkg + ".Request.LabelsEntry")},
			},
		},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Nested"),
			Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("id"), Number: proto.Int32(1), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()}},
		},
	)
	descriptor.EnumType = []*descriptorpb.EnumDescriptorProto{{
		Name:  proto.String("Kind"),
		Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("KIND_UNSPECIFIED"), Number: proto.Int32(0)}},
	}}
	for _, service := range file.services {
		serviceDescriptor := &descriptorpb.ServiceDescriptorProto{Name: proto.String(service.name)}
		if service.serviceDefault != nil {
			options := &descriptorpb.ServiceOptions{}
			proto.SetExtension(options, authzpb.E_ServiceDefault, service.serviceDefault)
			serviceDescriptor.Options = options
		}
		for _, method := range service.methods {
			methodDescriptor := &descriptorpb.MethodDescriptorProto{
				Name:       proto.String(method.name),
				InputType:  proto.String("." + file.protoPkg + ".Request"),
				OutputType: proto.String("." + file.protoPkg + ".Request"),
			}
			if method.rule != nil {
				options := &descriptorpb.MethodOptions{}
				proto.SetExtension(options, authzpb.E_Rule, method.rule)
				methodDescriptor.Options = options
			}
			serviceDescriptor.Method = append(serviceDescriptor.Method, methodDescriptor)
		}
		descriptor.Service = append(descriptor.Service, serviceDescriptor)
	}
	return descriptor
}

func checkRule(action, field string) *authzpb.AuthzRule {
	return &authzpb.AuthzRule{
		Mode:         authzpb.AuthzMode_AUTHZ_MODE_REQUIRED,
		Action:       action,
		ResourceType: "document",
		Target:       &authzpb.AuthzRule_ResourceIdField{ResourceIdField: field},
	}
}

func TestGenerateMergesOverridesSortsAndReturnsIndependentCopies(t *testing.T) {
	files := []fileSpec{{
		name:     "example/v1/service.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{
			name:           "ExampleService",
			serviceDefault: checkRule("read", "nested.id"),
			methods: []methodSpec{
				{name: "Zulu"},
				{name: "None", rule: &authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_NONE}},
				{name: "Alpha", rule: &authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_UNSPECIFIED, Action: "ignored"}},
			},
		}},
	}}
	firstPlugin, err := runPluginScenario(t, files)
	if err != nil {
		t.Fatalf("generate first: %v", err)
	}
	secondPlugin, err := runPluginScenario(t, files)
	if err != nil {
		t.Fatalf("generate second: %v", err)
	}
	first := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(firstPlugin), "authz_rules.gen.go")
	second := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(secondPlugin), "authz_rules.gen.go")
	if first != second {
		t.Fatal("same request produced different output")
	}
	alpha := strings.Index(first, `"/example.v1.ExampleService/Alpha"`)
	none := strings.Index(first, `"/example.v1.ExampleService/None"`)
	zulu := strings.Index(first, `"/example.v1.ExampleService/Zulu"`)
	if alpha < 0 || none <= alpha || zulu <= none {
		t.Fatalf("operations are not sorted: Alpha=%d None=%d Zulu=%d", alpha, none, zulu)
	}
	if strings.Contains(first, "ignored") || !strings.Contains(first, "AuthzMode_AUTHZ_MODE_NONE") || strings.Count(first, `"read"`) != 2 {
		t.Fatalf("inheritance or NONE override incorrect\n%s", first)
	}

	plugintest.AssertGeneratedGoTests(t, first, "examplev1", `package examplev1

import "testing"

func TestAuthzRulesIndependentCopies(t *testing.T) {
	const operation = "/example.v1.ExampleService/Alpha"
	first := AuthzRules()
	first[operation].Action = "mutated"
	delete(first, operation)
	second := AuthzRules()
	if second[operation] == nil || second[operation].Action != "read" {
		t.Fatalf("AuthzRules shared mutable state: %#v", second[operation])
	}
}
`)
}

func TestGenerateValidatesCheckRuleAndFieldPath(t *testing.T) {
	tests := []struct {
		name  string
		rule  *authzpb.AuthzRule
		match string
	}{
		{name: "missing action", rule: checkRule("", "id"), match: "requires action"},
		{name: "missing resource type", rule: &authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED, Action: "read", Target: &authzpb.AuthzRule_ResourceIdField{ResourceIdField: "id"}}, match: "requires resource_type"},
		{name: "missing target", rule: &authzpb.AuthzRule{Mode: authzpb.AuthzMode_AUTHZ_MODE_REQUIRED, Action: "read", ResourceType: "document"}, match: "exactly one non-empty resource target"},
		{name: "unknown field", rule: checkRule("read", "missing"), match: "not found"},
		{name: "empty segment", rule: checkRule("read", "nested..id"), match: "empty segment"},
		{name: "repeated field", rule: checkRule("read", "tags"), match: "repeated or map"},
		{name: "map field", rule: checkRule("read", "labels"), match: "repeated or map"},
		{name: "scalar intermediate", rule: checkRule("read", "id.value"), match: "not a message"},
		{name: "message terminal", rule: checkRule("read", "nested"), match: "unsupported terminal kind"},
		{name: "oneof field", rule: checkRule("read", "choice"), match: "belongs to oneof"},
		{name: "bool field", rule: checkRule("read", "enabled"), match: "unsupported terminal kind"},
		{name: "float field", rule: checkRule("read", "ratio"), match: "unsupported terminal kind"},
		{name: "bytes field", rule: checkRule("read", "payload"), match: "unsupported terminal kind"},
		{name: "enum field", rule: checkRule("read", "kind"), match: "unsupported terminal kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runPluginScenario(t, []fileSpec{{
				name:     "example/v1/bad.proto",
				protoPkg: "example.v1",
				goPkg:    "example.com/gen/example/v1;examplev1",
				generate: true,
				services: []serviceSpec{{name: "BadService", methods: []methodSpec{{name: "Get", rule: test.rule}}}},
			}})
			if err == nil || !strings.Contains(err.Error(), test.match) || !strings.Contains(err.Error(), "/example.v1.BadService/Get") {
				t.Fatalf("error = %v, want operation and %q", err, test.match)
			}
		})
	}
}

func TestGenerateAcceptsStaticAndIntegerTargets(t *testing.T) {
	static := &authzpb.AuthzRule{
		Mode:         authzpb.AuthzMode_AUTHZ_MODE_REQUIRED,
		Action:       "admin",
		ResourceType: "platform",
		Target:       &authzpb.AuthzRule_ResourceId{ResourceId: "default"},
	}
	plugin, err := runPluginScenario(t, []fileSpec{{
		name:     "example/v1/targets.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{name: "TargetService", methods: []methodSpec{
			{name: "Static", rule: static},
			{name: "Integer", rule: checkRule("read", "number")},
		}}},
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	content := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(plugin), "authz_rules.gen.go")
	if !strings.Contains(content, `ResourceId: "default"`) || !strings.Contains(content, `ResourceIdField: "number"`) {
		t.Fatalf("static or integer target missing\n%s", content)
	}
}

func TestGenerateProducesNoFileWithoutRules(t *testing.T) {
	plugin, err := runPluginScenario(t, []fileSpec{{
		name:     "example/v1/empty.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{name: "EmptyService", methods: []methodSpec{{name: "Get"}}}},
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if files := plugintest.ResponseFiles(plugin); len(files) != 0 {
		t.Fatalf("generated files = %v, want none", plugintest.SortedKeys(files))
	}
}

func TestGenerateRejectsConflictingPackageMetadata(t *testing.T) {
	_, err := runPluginScenario(t, []fileSpec{
		{name: "one.proto", protoPkg: "one", goPkg: "example.com/one;one", generate: true, services: []serviceSpec{{name: "One", methods: []methodSpec{{name: "Get", rule: checkRule("read", "id")}}}}},
		{name: "two.proto", protoPkg: "two", goPkg: "example.com/two;two", generate: true, services: []serviceSpec{{name: "Two", methods: []methodSpec{{name: "Get", rule: checkRule("read", "id")}}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting Go packages") {
		t.Fatalf("error = %v, want package conflict", err)
	}
}
