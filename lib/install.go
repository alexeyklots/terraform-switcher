package lib

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/mitchellh/go-homedir"
)

const (
	installFile               = "terraform"
	versionPrefix             = "terraform_"
	installPath               = ".terraform.versions"
	recentFile                = "RECENT"
	defaultBin                = "/usr/local/bin/terraform" //default bin installation dir
	tfDarwinArm64StartVersion = "1.0.2"
)

var (
	installLocation = "/tmp"
)

// initialize : removes existing symlink to terraform binary based on provided binPath
func initialize(binPath string) {

	/* find terraform binary location if terraform is already installed*/
	cmd := NewCommand(binPath)
	next := cmd.Find()

	/* override installation default binary path if terraform is already installed */
	/* find the last bin path */
	for path := next(); len(path) > 0; path = next() {
		binPath = path
	}

	/* check if current symlink to terraform binary exist */
	symlinkExist := CheckSymlink(binPath)

	/* remove current symlink if exist*/
	if symlinkExist {
		RemoveSymlink(binPath)
	}

}

// GetInstallLocation : get location where the terraform binary will be installed,
// will create a directory in the home location if it does not exist
func GetInstallLocation() string {
	/* get current user */
	homedir, errCurr := homedir.Dir()
	if errCurr != nil {
		log.Fatal(errCurr)
	}

	userCommon := homedir

	/* set installation location */
	installLocation = filepath.Join(userCommon, installPath)

	/* Create local installation directory if it does not exist */
	CreateDirIfNotExist(installLocation)

	return installLocation

}

// Install : Install the provided version in the argument
func Install(tfversion string, binPath string, mirrorURL string) {

	// if !ValidVersionFormat(tfversion) {
	// 	fmt.Printf("The provided terraform version format does not exist - %s. Try `tfswitch -l` to see all available versions.\n", tfversion)
	// 	os.Exit(1)
	// }

	/* Check to see if user has permission to the default bin location which is  "/usr/local/bin/terraform"
	 * If user does not have permission to default bin location, proceed to create $HOME/bin and install the tfswitch there
	 * Inform user that they don't have permission to default location, therefore tfswitch was installed in $HOME/bin
	 * Tell users to add $HOME/bin to their path
	 */
	binPath = InstallableBinLocation(binPath)

	initialize(binPath)                    //initialize path
	installLocation = GetInstallLocation() //get installation location -  this is where we will put our terraform binary file

	goarch := runtime.GOARCH
	goos := runtime.GOOS

	// Terraform darwin arm64 comes with 1.0.2 and next version
	tfver, _ := version.NewVersion(tfversion)
	tf102, _ := version.NewVersion(tfDarwinArm64StartVersion)
	if goos == "darwin" && goarch == "arm64" && tfver.LessThan(tf102) {
		goarch = "amd64"
	}

	/* check if selected version already downloaded */
	installFileVersionPath := ConvertExecutableExt(filepath.Join(installLocation, versionPrefix+tfversion))
	fileExist := CheckFileExist(installFileVersionPath)

	/* if selected version already exist, */
	if fileExist {

		/* remove current symlink if exist*/
		symlinkExist := CheckSymlink(binPath)

		if symlinkExist {
			RemoveSymlink(binPath)
		}

		/* set symlink to desired version */
		CreateSymlink(installFileVersionPath, binPath)
		fmt.Printf("Switched terraform to version %q \n", tfversion)
		AddRecent(tfversion) //add to recent file for faster lookup
		os.Exit(0)
	}

	//if does not have slash - append slash
	hasSlash := strings.HasSuffix(mirrorURL, "/")
	if !hasSlash {
		mirrorURL = fmt.Sprintf("%s/", mirrorURL)
	}

	/* if selected version already exist, */
	/* proceed to download it from the hashicorp release page */
	url := mirrorURL + tfversion + "/" + versionPrefix + tfversion + "_" + goos + "_" + goarch + ".zip"
	zipFile, errDownload := DownloadFromURL(installLocation, url)

	/* If unable to download file from url, exit(1) immediately */
	if errDownload != nil {
		fmt.Println(errDownload)
		os.Exit(1)
	}

	/* unzip the downloaded zipfile */
	_, errUnzip := Unzip(zipFile, installLocation)
	if errUnzip != nil {
		fmt.Println("[Error] : Unable to unzip downloaded zip file")
		log.Fatal(errUnzip)
		os.Exit(1)
	}

	/* rename unzipped file to terraform version name - terraform_x.x.x */
	installFilePath := ConvertExecutableExt(filepath.Join(installLocation, installFile))
	RenameFile(installFilePath, installFileVersionPath)

	/* remove zipped file to clear clutter */
	RemoveFiles(zipFile)

	/* remove current symlink if exist*/
	symlinkExist := CheckSymlink(binPath)

	if symlinkExist {
		RemoveSymlink(binPath)
	}

	/* set symlink to desired version */
	CreateSymlink(installFileVersionPath, binPath)
	fmt.Printf("Switched terraform to version %q \n", tfversion)
	AddRecent(tfversion) //add to recent file for faster lookup
	os.Exit(0)
}

// AddRecent : add to recent file
func AddRecent(requestedVersion string) {

	installLocation = GetInstallLocation() //get installation location -  this is where we will put our terraform binary file
	versionFile := filepath.Join(installLocation, recentFile)

	fileExist := CheckFileExist(versionFile)
	if fileExist {
		lines, errRead := ReadLines(versionFile)

		if errRead != nil {
			fmt.Printf("[Error] : %s\n", errRead)
			return
		}

		for _, line := range lines {
			if !ValidVersionFormat(line) {
				fmt.Println("File dirty. Recreating cache file.")
				RemoveFiles(versionFile)
				CreateRecentFile(requestedVersion)
				return
			}
		}

		versionExist := VersionExist(requestedVersion, lines)

		if !versionExist {
			if len(lines) >= 3 {
				_, lines = lines[len(lines)-1], lines[:len(lines)-1]

				lines = append([]string{requestedVersion}, lines...)
				WriteLines(lines, versionFile)
			} else {
				lines = append([]string{requestedVersion}, lines...)
				WriteLines(lines, versionFile)
			}
		}

	} else {
		CreateRecentFile(requestedVersion)
	}
}

// GetRecentVersions : get recent version from file
func GetRecentVersions() ([]string, error) {

	installLocation = GetInstallLocation() //get installation location -  this is where we will put our terraform binary file
	versionFile := filepath.Join(installLocation, recentFile)

	fileExist := CheckFileExist(versionFile)
	if fileExist {

		lines, errRead := ReadLines(versionFile)
		outputRecent := []string{}

		if errRead != nil {
			fmt.Printf("Error: %s\n", errRead)
			return nil, errRead
		}

		for _, line := range lines {
			/* 	checks if versions in the recent file are valid.
			If any version is invalid, it will be considered dirty
			and the recent file will be removed
			*/
			if !ValidVersionFormat(line) {
				RemoveFiles(versionFile)
				return nil, errRead
			}

			/* 	output can be confusing since it displays the 3 most recent used terraform version
			append the string *recent to the output to make it more user friendly
			*/
			outputRecent = append(outputRecent, fmt.Sprintf("%s *recent", line))
		}

		return outputRecent, nil
	}

	return nil, nil
}

// CreateRecentFile : create a recent file
func CreateRecentFile(requestedVersion string) {

	installLocation = GetInstallLocation() //get installation location -  this is where we will put our terraform binary file

	WriteLines([]string{requestedVersion}, filepath.Join(installLocation, recentFile))
}

// ConvertExecutableExt : convert excutable with local OS extension
func ConvertExecutableExt(fpath string) string {
	switch runtime.GOOS {
	case "windows":
		if filepath.Ext(fpath) == ".exe" {
			return fpath
		}
		return fpath + ".exe"
	default:
		return fpath
	}
}

// InstallableBinLocation : Checks if terraform is installable in the location provided by the user.
// If not, create $HOME/bin. Ask users to add  $HOME/bin to $PATH and return $HOME/bin as install location
func InstallableBinLocation(userBinPath string) string {

	homedir, errCurr := homedir.Dir()
	if errCurr != nil {
		log.Fatal(errCurr)
	}

	binDir := Path(userBinPath)           //get path directory from binary path
	binPathExist := CheckDirExist(binDir) //the default is /usr/local/bin but users can provide custom bin locations

	if binPathExist == true { //if bin path exist - check if we can write to it

		binPathWritable := false //assume bin path is not writable
		if runtime.GOOS != "windows" {
			binPathWritable = CheckDirWritable(binDir) //check if is writable on ( only works on LINUX)
		}

		// IF: "/usr/local/bin" or `custom bin path` provided by user is non-writable, (binPathWritable == false), we will attempt to install terraform at the ~/bin location. See ELSE
		if !binPathWritable {

			homeBinExist := CheckDirExist(filepath.Join(homedir, "bin")) //check to see if ~/bin exist
			if homeBinExist {                                            //if ~/bin exist, install at ~/bin/terraform
				fmt.Printf("Installing terraform at %s\n", filepath.Join(homedir, "bin"))
				return filepath.Join(homedir, "bin", "terraform")
			} else { //if ~/bin directory does not exist, create ~/bin for terraform installation
				fmt.Printf("Unable to write to: %s\n", userBinPath)
				fmt.Printf("Creating bin directory at: %s\n", filepath.Join(homedir, "bin"))
				CreateDirIfNotExist(filepath.Join(homedir, "bin")) //create ~/bin
				fmt.Printf("RUN `export PATH=$PATH:%s` to append bin to $PATH\n", filepath.Join(homedir, "bin"))
				return filepath.Join(homedir, "bin", "terraform")
			}
		} else { // ELSE: the "/usr/local/bin" or custom path provided by user is writable, we will return installable location
			return filepath.Join(userBinPath)
		}
	}
	fmt.Printf("[Error] : Binary path does not exist: %s\n", userBinPath)
	fmt.Printf("[Error] : Manually create bin directory at: %s and try again.\n", binDir)
	os.Exit(1)
	return ""
}
