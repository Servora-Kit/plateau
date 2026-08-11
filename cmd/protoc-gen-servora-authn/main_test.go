package main

import (
	"strings"
	"testing"

	authnpb "github.com/Servora-Kit/servora-platform/api/gen/go/platform/authn/v1"
	"github.com/Servora-Kit/servora-platform/internal/codegen/plugintest"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type methodSpec struct {
	name string
	rule *authnpb.AuthnRule
}

type serviceSpec struct {
	name           string
	serviceDefault *authnpb.AuthnRule
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
		ProtoFile: plugintest.DescriptorClosure(authnpb.File_platform_authn_v1_annotations_proto),
		Parameter: proto.String("paths=source_relative"),
	}
	for _, file := range files {
		request.ProtoFile = append(request.ProtoFile, authnFileDescriptor(file))
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

func authnFileDescriptor(file fileSpec) *descriptorpb.FileDescriptorProto {
	descriptor := &descriptorpb.FileDescriptorProto{
		Name:       proto.String(file.name),
		Package:    proto.String(file.protoPkg),
		Syntax:     proto.String(protoreflect.Proto3.String()),
		Dependency: []string{"google/protobuf/descriptor.proto", "platform/authn/v1/annotations.proto"},
		Options:    &descriptorpb.FileOptions{GoPackage: proto.String(file.goPkg)},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Empty"),
		}},
	}
	for _, service := range file.services {
		serviceDescriptor := &descriptorpb.ServiceDescriptorProto{Name: proto.String(service.name)}
		if service.serviceDefault != nil {
			options := &descriptorpb.ServiceOptions{}
			proto.SetExtension(options, authnpb.E_ServiceDefault, service.serviceDefault)
			serviceDescriptor.Options = options
		}
		for _, method := range service.methods {
			methodDescriptor := &descriptorpb.MethodDescriptorProto{
				Name:       proto.String(method.name),
				InputType:  proto.String("." + file.protoPkg + ".Empty"),
				OutputType: proto.String("." + file.protoPkg + ".Empty"),
			}
			if method.rule != nil {
				options := &descriptorpb.MethodOptions{}
				proto.SetExtension(options, authnpb.E_Rule, method.rule)
				methodDescriptor.Options = options
			}
			serviceDescriptor.Method = append(serviceDescriptor.Method, methodDescriptor)
		}
		descriptor.Service = append(descriptor.Service, serviceDescriptor)
	}
	return descriptor
}

func TestGenerateMergesOverridesSortsAndReturnsIndependentCopies(t *testing.T) {
	files := []fileSpec{{
		name:     "example/v1/service.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{
			name: "ExampleService",
			serviceDefault: &authnpb.AuthnRule{
				Mode:    authnpb.AuthnMode_AUTHN_MODE_REQUIRED,
				Schemes: []string{"jwt", "mtls"},
			},
			methods: []methodSpec{
				{name: "Zulu"},
				{name: "Public", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC}},
				{name: "Alpha", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_UNSPECIFIED}},
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
	first := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(firstPlugin), "authn_rules.gen.go")
	second := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(secondPlugin), "authn_rules.gen.go")
	if first != second {
		t.Fatal("same request produced different output")
	}
	alpha := strings.Index(first, `"/example.v1.ExampleService/Alpha"`)
	public := strings.Index(first, `"/example.v1.ExampleService/Public"`)
	zulu := strings.Index(first, `"/example.v1.ExampleService/Zulu"`)
	if alpha < 0 || public <= alpha || zulu <= public {
		t.Fatalf("operations are not sorted: Alpha=%d Public=%d Zulu=%d", alpha, public, zulu)
	}
	if strings.Count(first, `"jwt"`) != 2 || !strings.Contains(first, "AuthnMode_AUTHN_MODE_PUBLIC") {
		t.Fatalf("inheritance or PUBLIC override missing\n%s", first)
	}

	plugintest.AssertGeneratedGoTests(t, first, "examplev1", `package examplev1

import "testing"

func TestAuthnRulesIndependentCopies(t *testing.T) {
	const operation = "/example.v1.ExampleService/Alpha"
	first := AuthnRules()
	first[operation].Schemes[0] = "mutated"
	delete(first, operation)
	second := AuthnRules()
	if second[operation] == nil || second[operation].Schemes[0] != "jwt" {
		t.Fatalf("AuthnRules shared mutable state: %#v", second[operation])
	}
}
`)
}

func TestGenerateAcceptsRequiredWithEmptySchemes(t *testing.T) {
	plugin, err := runPluginScenario(t, []fileSpec{{
		name:     "example/v1/service.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{
			name:    "ExampleService",
			methods: []methodSpec{{name: "Get", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_REQUIRED}}},
		}},
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	content := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(plugin), "authn_rules.gen.go")
	if !strings.Contains(content, "AuthnMode_AUTHN_MODE_REQUIRED") || strings.Contains(content, "Schemes:") {
		t.Fatalf("unexpected REQUIRED empty-schemes output\n%s", content)
	}
}

func TestGenerateRejectsIllegalModeSchemesCombinations(t *testing.T) {
	tests := []struct {
		name string
		rule *authnpb.AuthnRule
		want string
	}{
		{name: "unspecified", rule: &authnpb.AuthnRule{Schemes: []string{"jwt"}}, want: "AUTHN_MODE_UNSPECIFIED"},
		{name: "public", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC, Schemes: []string{"jwt"}}, want: "AUTHN_MODE_PUBLIC"},
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
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "BadService") {
				t.Fatalf("error = %v, want mode %q and service", err, test.want)
			}
		})
	}
}

func TestGenerateRejectsConflictingPackageMetadata(t *testing.T) {
	_, err := runPluginScenario(t, []fileSpec{
		{name: "one.proto", protoPkg: "one", goPkg: "example.com/one;one", generate: true, services: []serviceSpec{{name: "One", methods: []methodSpec{{name: "Get", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC}}}}}},
		{name: "two.proto", protoPkg: "two", goPkg: "example.com/two;two", generate: true, services: []serviceSpec{{name: "Two", methods: []methodSpec{{name: "Get", rule: &authnpb.AuthnRule{Mode: authnpb.AuthnMode_AUTHN_MODE_PUBLIC}}}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting Go packages") {
		t.Fatalf("error = %v, want package conflict", err)
	}
}
