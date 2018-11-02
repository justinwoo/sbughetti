package main

import "flag"
import "fmt"
import "os"
import "os/exec"
import "strings"
import "runtime"
import "sync"

const dir string = ".spacchetti"
const config string = dir + "/spacchetti.json"

func ensureSpacchettiConfig() {
	_, existsErr := os.Stat(config)

	if existsErr == nil {
		return
	}

	cmd := exec.Command(
		"bash",
		"-c",
		"dhall-to-json <<< ./spacchetti.dhall > "+config,
	)

	execErr := cmd.Run()

	if execErr != nil {
		panic(execErr)
	}
}

func jqConfig(expression string) string {
	cmd := exec.Command(
		"jq",
		expression,
		"-r",
		config,
	)

	output, execErr := cmd.CombinedOutput()

	if execErr != nil {
		panic(execErr)
	}

	return string(output)
}

func getKeys(values map[string]bool) []string {
	// i dont know where this is in the stdlib
	keys := make([]string, len(values))

	i := 0
	for k := range values {
		keys[i] = k
		i++
	}

	return keys
}

func mkdirAll(path string) {
	weirdErr := os.MkdirAll(path, 0755)

	if weirdErr != nil {
		panic(weirdErr)
	}
}

func getProperty(dep string, property string) string {
	result := jqConfig(".packages.\"" + dep + "\".\"" + property + "\"")
	return strings.TrimSpace(result)
}

func getTransitiveDepsOf(dep string, visited map[string]bool) {
	if visited[dep] == true {
		return
	}

	visited[dep] = true

	output := jqConfig(".packages.\"" + dep + "\".dependencies[]")

	xs := strings.Fields(output)

	for _, x := range xs {
		getTransitiveDepsOf(x, visited)
	}
}

func getDeps() []string {
	output := jqConfig(".dependencies[]")

	directDeps := strings.Fields(output)

	var visited = make(map[string]bool)

	for _, dep := range directDeps {
		getTransitiveDepsOf(dep, visited)
	}

	keys := getKeys(visited)

	return keys
}

func getTargetDir(dep string, version string) string {
	return fmt.Sprintf("%s/%s/%s", dir, dep, version)
}

func installDep(dep string) {
	repo := getProperty(dep, "repo")
	version := getProperty(dep, "version")

	targetDir := getTargetDir(dep, version)

	_, existsErr := os.Stat(targetDir)

	if existsErr == nil {
		return
	}

	fmt.Println("installing", targetDir)

	cmd := exec.Command(
		"git",
		"clone",
		"-c",
		"advice.detachedHead=false",
		"--branch",
		version,
		repo,
		targetDir,
	)

	err := cmd.Run()

	if err != nil {
		panic(err)
	}
}

func getSources() []string {
	ensureSpacchettiConfig()
	deps := getDeps()

	var globs = make(map[string]bool)

	for _, dep := range deps {
		version := getProperty(dep, "version")
		targetDir := getTargetDir(dep, version)
		glob := fmt.Sprintf("%s/src/**/*.purs", targetDir)
		globs[glob] = true
	}

	keys := getKeys(globs)

	return keys
}

func main() {
	var install bool
	var sources bool
	var build bool

	flag.BoolVar(&install, "install", false, "install the project dependencies.")
	flag.BoolVar(&sources, "sources", false, "get the source globs for the project.")
	flag.BoolVar(&build, "build", false, "build the project.")
	flag.Parse()

	// make sure we have dir to work with
	mkdirAll(dir)

	if install {
		ensureSpacchettiConfig()
		deps := getDeps()

		fmt.Println("Installing", len(deps), "dependencies.")

		runtime.GOMAXPROCS(10)

		var wg sync.WaitGroup
		wg.Add(len(deps))

		for _, dep := range deps {
			go func(dep string) {
				defer wg.Done()

				installDep(dep)
			}(dep)
		}

		wg.Wait()

		fmt.Println("Installed dependencies.")
	}
	if sources {
		sources := getSources()

		for _, x := range sources {
			fmt.Println(x)
		}
	}
	if build {
		sources := getSources()

		baseArgs := []string{"compile", "src/**/*.purs", "test/**/*.purs"}

		args := append(baseArgs, sources...)

		cmd := exec.Command(
			"purs",
			args...,
		)

		execErr := cmd.Run()

		if execErr != nil {
			panic(execErr)
		}

		fmt.Println("Build succeeded.")
	}

	if !install && !sources && !build {
		fmt.Println("You must specify at least one task to run. See -help.")
		os.Exit(1)
	}
}
