package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/sprig"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// autogenerationMessage is message added at the beginning of each autogenerated file.
	autogenerationMessage = "Code generated by rendertemplates. DO NOT EDIT."
)

var (
	configFilePath  = flag.String("config", "", "Path of the config file")
	additionalFuncs = map[string]interface{}{
		"matchingReleases": matchingReleases,
		"releaseMatches":   releaseMatches,
	}
	commentSignByFileExt = map[string]sets.String{
		"//": sets.NewString(".go"),
		"#":  sets.NewString(".yaml", ".yml"),
	}
)

// Config represents configuration of all templates to render along with global values
type Config struct {
	Templates []TemplateConfig
	Global    map[string]interface{}
}

// TemplateConfig specifies template to use and files to render
type TemplateConfig struct {
	From   string
	Render []RenderConfig
}

// RenderConfig specifies where to render template and values to use
type RenderConfig struct {
	To     string
	Values map[string]interface{}
}

func main() {
	flag.Parse()

	if *configFilePath == "" {
		log.Fatal("Provide path to config file with --config")
	}

	configFile, err := ioutil.ReadFile(*configFilePath)
	if err != nil {
		log.Fatalf("Cannot read config file: %s", err)
	}

	config := new(Config)
	err = yaml.Unmarshal(configFile, config)
	if err != nil {
		log.Fatalf("Cannot parse config yaml: %s\n", err)
	}

	for _, templateConfig := range config.Templates {
		err = renderTemplate(path.Dir(*configFilePath), templateConfig, config)
		if err != nil {
			log.Fatalf("Cannot render template %s: %s", templateConfig.From, err)
		}
	}
}

func renderTemplate(basePath string, templateConfig TemplateConfig, config *Config) error {
	templateInstance, err := loadTemplate(basePath, templateConfig.From)
	if err != nil {
		return err
	}

	for _, render := range templateConfig.Render {
		err = renderFileFromTemplate(basePath, templateInstance, render, config)
		if err != nil {
			return err
		}
	}

	return nil
}

func renderFileFromTemplate(basePath string, templateInstance *template.Template, renderConfig RenderConfig, config *Config) error {
	relativeDestPath := path.Join(basePath, renderConfig.To)

	destDir := path.Dir(relativeDestPath)
	err := os.MkdirAll(destDir, os.ModePerm)
	if err != nil {
		return err
	}

	destFile, err := os.Create(relativeDestPath)
	if err != nil {
		return err
	}

	tmplPath := fmt.Sprintf("templates%s%s%s%s", string(os.PathSeparator), basePath, string(os.PathSeparator), templateInstance.Name())

	if err := addAutogeneratedHeader(destFile, tmplPath); err != nil {
		return err
	}

	values := map[string]interface{}{"Values": renderConfig.Values, "Global": config.Global}

	return templateInstance.Execute(destFile, values)
}

func loadTemplate(basePath, templatePath string) (*template.Template, error) {
	relativeTemplatePath := path.Join(basePath, templatePath)
	return template.
		New(path.Base(templatePath)).
		Funcs(sprig.TxtFuncMap()).
		Funcs(additionalFuncs).
		ParseFiles(relativeTemplatePath)
}

func matchingReleases(allReleases []interface{}, since interface{}, until interface{}) []interface{} {
	result := make([]interface{}, 0)
	for _, rel := range allReleases {
		if releaseMatches(rel, since, until) {
			result = append(result, rel)
		}
	}
	return result
}

func releaseMatches(rel interface{}, since interface{}, until interface{}) bool {
	relVer := semver.MustParse(rel.(string))
	if since != nil && relVer.Compare(semver.MustParse(since.(string))) < 0 {
		return false
	}
	if until != nil && relVer.Compare(semver.MustParse(until.(string))) > 0 {
		return false
	}
	return true
}

func addAutogeneratedHeader(destFile *os.File, tmplPath string) error {
	outputExt := filepath.Ext(destFile.Name())
	sign, err := commentSign(outputExt)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("%s %s \n%s Edit template instead: %s\n\n", sign, autogenerationMessage, sign, tmplPath)
	if _, err := destFile.WriteString(header); err != nil {
		return err
	}

	return nil
}

func commentSign(extension string) (string, error) {
	for sign, extFile := range commentSignByFileExt {
		if extFile.Has(extension) {
			return sign, nil
		}
	}
	return "", fmt.Errorf("cannot add autogenerated header comment: unknow comment sign for %q file extension", extension)
}
