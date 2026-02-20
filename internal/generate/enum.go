package generate

import (
	"strings"

	"github.com/dave/jennifer/jen"
	annotations "github.com/drewfead/proto-cli/proto/cli/v1"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// generateEnumParser generates a helper function to parse enum values from strings
func generateEnumParser(f *jen.File, service *protogen.Service, enum *protogen.Enum) {
	enumTypeName := enum.GoIdent.GoName
	parserFuncName := enumParserFuncName(service, enumTypeName)

	f.Commentf("%s parses a string value to %s enum", parserFuncName, enumTypeName)
	f.Commentf("Accepts enum value names (case-insensitive) or custom CLI names if specified")
	f.Func().Id(parserFuncName).Params(
		jen.Id("value").String(),
	).Params(jen.Id(enumTypeName), jen.Error()).Block(
		jen.Comment("Convert to lowercase for case-insensitive comparison"),
		jen.Id("lower").Op(":=").Qual("strings", "ToLower").Call(jen.Id("value")),
		jen.Line(),
		jen.Comment("Try parsing as enum value name or custom CLI name"),
		jen.Switch(jen.Id("lower")).Block(
			generateEnumParserCases(enum)...,
		),
		jen.Line(),
		jen.Comment("Try parsing as number"),
		jen.List(jen.Id("num"), jen.Err()).Op(":=").Qual("strconv", "ParseInt").Call(
			jen.Id("value"),
			jen.Lit(10),
			jen.Lit(32),
		),
		jen.If(jen.Err().Op("==").Nil()).Block(
			jen.Return(
				jen.Id(enumTypeName).Call(jen.Id("num")),
				jen.Nil(),
			),
		),
		jen.Line(),
		jen.Comment("Invalid value"),
		jen.Return(
			jen.Lit(0),
			jen.Qual("fmt", "Errorf").Call(
				jen.Lit("invalid %s value: %q (valid values: %s)"),
				jen.Lit(enumTypeName),
				jen.Id("value"),
				jen.Lit(getEnumValidValues(enum)),
			),
		),
	)
	f.Line()
}

// generateEnumParserCases generates switch cases for enum parser
func generateEnumParserCases(enum *protogen.Enum) []jen.Code {
	var cases []jen.Code

	for _, value := range enum.Values {
		// Skip the unspecified/zero value
		if value.Desc.Number() == 0 {
			continue
		}

		// Get custom CLI name from annotation if present
		customName := getEnumValueCLIName(value)

		// Add case for enum value name (lowercase)
		valueName := string(value.Desc.Name())
		caseValues := []jen.Code{jen.Lit(strings.ToLower(valueName))}

		// Add case for custom CLI name if different from value name
		if customName != "" && !strings.EqualFold(customName, valueName) {
			caseValues = append(caseValues, jen.Lit(strings.ToLower(customName)))
		}

		cases = append(cases,
			jen.Case(caseValues...).Block(
				jen.Return(
					jen.Id(value.GoIdent.GoName),
					jen.Nil(),
				),
			),
		)
	}

	return cases
}

// getEnumValueCLIName extracts the custom CLI name from enum value annotation
func getEnumValueCLIName(value *protogen.EnumValue) string {
	opts := value.Desc.Options()
	if opts == nil {
		return ""
	}

	if !proto.HasExtension(opts, annotations.E_EnumValue) {
		return ""
	}

	ext := proto.GetExtension(opts, annotations.E_EnumValue)
	if ext == nil {
		return ""
	}

	enumValueOpts, ok := ext.(*annotations.EnumValueOptions)
	if !ok || enumValueOpts == nil {
		return ""
	}

	return enumValueOpts.Name
}

// getEnumValuesPiped returns a pipe-separated list of valid enum values for usage text (e.g., "debug|info|warn|error")
func getEnumValuesPiped(enum *protogen.Enum) string {
	var values []string
	for _, value := range enum.Values {
		if value.Desc.Number() == 0 {
			continue
		}
		customName := getEnumValueCLIName(value)
		if customName != "" {
			values = append(values, customName)
		} else {
			values = append(values, strings.ToLower(string(value.Desc.Name())))
		}
	}
	return strings.Join(values, "|")
}

// getEnumValidValues returns a comma-separated list of valid enum values for error messages
func getEnumValidValues(enum *protogen.Enum) string {
	var values []string
	for _, value := range enum.Values {
		if value.Desc.Number() == 0 {
			continue
		}

		// Use custom CLI name if available, otherwise use enum value name
		customName := getEnumValueCLIName(value)
		if customName != "" {
			values = append(values, customName)
		} else {
			values = append(values, strings.ToLower(string(value.Desc.Name())))
		}
	}
	return strings.Join(values, ", ")
}
