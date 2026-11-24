package main

import (
	"strings"
	"testing"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
	"github.com/Servora-Kit/plateau/internal/codegen/plugintest"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type methodSpec struct {
	name string
	rule *auditpb.AuditRule
}

type serviceSpec struct {
	name           string
	serviceDefault *auditpb.AuditRule
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
		ProtoFile: plugintest.DescriptorClosure(auditpb.File_plateau_security_audit_v1_annotations_proto),
		Parameter: new("paths=source_relative"),
	}
	for _, file := range files {
		request.ProtoFile = append(request.ProtoFile, auditFileDescriptor(file))
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

func auditFileDescriptor(file fileSpec) *descriptorpb.FileDescriptorProto {
	descriptor := &descriptorpb.FileDescriptorProto{
		Name:       new(file.name),
		Package:    new(file.protoPkg),
		Syntax:     new(protoreflect.Proto3.String()),
		Dependency: []string{"google/protobuf/descriptor.proto", "plateau/security/audit/v1/annotations.proto"},
		Options:    &descriptorpb.FileOptions{GoPackage: new(file.goPkg)},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Empty"),
		}},
	}
	for _, service := range file.services {
		serviceDescriptor := &descriptorpb.ServiceDescriptorProto{Name: new(service.name)}
		if service.serviceDefault != nil {
			options := &descriptorpb.ServiceOptions{}
			proto.SetExtension(options, auditpb.E_ServiceDefault, service.serviceDefault)
			serviceDescriptor.Options = options
		}
		for _, method := range service.methods {
			methodDescriptor := &descriptorpb.MethodDescriptorProto{
				Name:       new(method.name),
				InputType:  new("." + file.protoPkg + ".Empty"),
				OutputType: new("." + file.protoPkg + ".Empty"),
			}
			if method.rule != nil {
				options := &descriptorpb.MethodOptions{}
				proto.SetExtension(options, auditpb.E_Rule, method.rule)
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
			serviceDefault: &auditpb.AuditRule{
				Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
				Category: auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE,
			},
			methods: []methodSpec{
				{name: "Zulu"},
				{name: "Read", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode_AUDIT_MODE_DISABLED}},
				{name: "Login", rule: &auditpb.AuditRule{
					Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
					Category: auditpb.AuditCategory_AUDIT_CATEGORY_AUTHN,
				}},
				{name: "Alpha", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode_AUDIT_MODE_UNSPECIFIED}},
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
	first := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(firstPlugin), "audit_rules.gen.go")
	second := plugintest.OnlyGeneratedFile(t, plugintest.ResponseFiles(secondPlugin), "audit_rules.gen.go")
	if first != second {
		t.Fatal("same request produced different output")
	}

	alpha := strings.Index(first, `"/example.v1.ExampleService/Alpha"`)
	login := strings.Index(first, `"/example.v1.ExampleService/Login"`)
	read := strings.Index(first, `"/example.v1.ExampleService/Read"`)
	zulu := strings.Index(first, `"/example.v1.ExampleService/Zulu"`)
	if alpha < 0 || login <= alpha || read <= login || zulu <= read {
		t.Fatalf("operations are not sorted: Alpha=%d Login=%d Read=%d Zulu=%d", alpha, login, read, zulu)
	}
	if !strings.Contains(first, "AuditMode_AUDIT_MODE_DISABLED") ||
		!strings.Contains(first, "AuditCategory_AUDIT_CATEGORY_RESOURCE") ||
		!strings.Contains(first, "AuditCategory_AUDIT_CATEGORY_AUTHN") {
		t.Fatalf("expected modes or categories missing\n%s", first)
	}

	plugintest.AssertGeneratedGoTests(t, first, "examplev1", `package examplev1

import (
	"testing"

	auditpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/audit/v1"
)

func TestAuditRulesIndependentCopies(t *testing.T) {
	const alphaOp = "/example.v1.ExampleService/Alpha"
	const readOp = "/example.v1.ExampleService/Read"

	first := AuditRules()
	if first[alphaOp].GetMode() != auditpb.AuditMode_AUDIT_MODE_ENABLED ||
		first[alphaOp].GetCategory() != auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE {
		t.Fatalf("Alpha did not inherit service_default: %#v", first[alphaOp])
	}
	if first[readOp].GetMode() != auditpb.AuditMode_AUDIT_MODE_DISABLED ||
		first[readOp].GetCategory() != auditpb.AuditCategory_AUDIT_CATEGORY_UNSPECIFIED {
		t.Fatalf("Read override leaked service_default category: %#v", first[readOp])
	}

	first[alphaOp].Mode = auditpb.AuditMode_AUDIT_MODE_DISABLED
	first[alphaOp].Category = auditpb.AuditCategory_AUDIT_CATEGORY_AUTHZ
	delete(first, alphaOp)

	second := AuditRules()
	if second[alphaOp] == nil ||
		second[alphaOp].GetMode() != auditpb.AuditMode_AUDIT_MODE_ENABLED ||
		second[alphaOp].GetCategory() != auditpb.AuditCategory_AUDIT_CATEGORY_RESOURCE {
		t.Fatalf("AuditRules shared mutable state: %#v", second[alphaOp])
	}
}
`)
}

func TestGenerateSkipsFileWithoutRules(t *testing.T) {
	plugin, err := runPluginScenario(t, []fileSpec{{
		name:     "example/v1/plain.proto",
		protoPkg: "example.v1",
		goPkg:    "example.com/gen/example/v1;examplev1",
		generate: true,
		services: []serviceSpec{{
			name:    "PlainService",
			methods: []methodSpec{{name: "Ping"}},
		}},
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if files := plugintest.ResponseFiles(plugin); len(files) != 0 {
		t.Fatalf("expected no generated files, got %#v", files)
	}
}

func TestGenerateRejectsUnknownEnums(t *testing.T) {
	tests := []struct {
		name    string
		service serviceSpec
		want    string
	}{
		{
			name: "unknown mode on method",
			service: serviceSpec{
				name:    "BadModeService",
				methods: []methodSpec{{name: "Save", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode(99)}}},
			},
			want: "unknown AuditMode",
		},
		{
			name: "unknown category on overridden service default",
			service: serviceSpec{
				name: "BadCategoryService",
				serviceDefault: &auditpb.AuditRule{
					Mode:     auditpb.AuditMode_AUDIT_MODE_ENABLED,
					Category: auditpb.AuditCategory(77),
				},
				methods: []methodSpec{{name: "Save", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode_AUDIT_MODE_DISABLED}}},
			},
			want: "unknown AuditCategory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runPluginScenario(t, []fileSpec{{
				name:     "example/v1/bad.proto",
				protoPkg: "example.v1",
				goPkg:    "example.com/gen/example/v1;examplev1",
				generate: true,
				services: []serviceSpec{tc.service},
			}})
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.service.name) {
				t.Fatalf("error = %v, want %q and service %q", err, tc.want, tc.service.name)
			}
		})
	}
}

func TestGenerateRejectsConflictingPackageMetadata(t *testing.T) {
	_, err := runPluginScenario(t, []fileSpec{
		{
			name:     "one.proto",
			protoPkg: "one",
			goPkg:    "example.com/one;one",
			generate: true,
			services: []serviceSpec{{name: "One", methods: []methodSpec{{name: "Save", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode_AUDIT_MODE_ENABLED}}}}},
		},
		{
			name:     "two.proto",
			protoPkg: "two",
			goPkg:    "example.com/two;two",
			generate: true,
			services: []serviceSpec{{name: "Two", methods: []methodSpec{{name: "Save", rule: &auditpb.AuditRule{Mode: auditpb.AuditMode_AUDIT_MODE_ENABLED}}}}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting Go packages") {
		t.Fatalf("error = %v, want package conflict", err)
	}
}
