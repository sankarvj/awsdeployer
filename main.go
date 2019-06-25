package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/otiai10/copy"
	"github.com/spf13/viper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/codedeploy"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pierrre/archivefile/zip"
)

var (
	awsSession *session.Session
)

const (
	nexusBaseURL        = "<give nexus url here>"
	nexusBaseURLWithPwd = "<give nexus url here with password>"
	groupName           = "< give com.example.product>"
	s3Region            = "us-east-1"
	javaAppPath         = "/tmp/codedeploy/java-app/"
	javaAppSourcePath   = "/tmp/codedeploy/java-app/content/ROOT.war"
)

//RequestBody is the request body
type RequestBody struct {
	Environment string `json:"environment"` //playground,staging,prod
	Version     string `json:"version"`     //artifact version
	Instance    string `json:"instance"`    //remove this once codedeploy is done
	Artifactid  string `json:"artifactid"`  //artifact id
	Codedeploy  bool   `json:"codedeploy"`  //remove this once codedeploy is done
	Product     string `json:"product"`
}

func main() {
	lambda.Start(Handler)
}

//Handler is the starting point of lamda function
func Handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	reqBody, err := reqBodyDecode(request.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       "Could not parse request body",
			StatusCode: 500,
		}, nil
	}
	log.Println("reqBody :: ", reqBody)

	//NOTE:: Always do this in first place; Can I use once?
	instance, err := configInstance(reqBody.Environment, reqBody.Product)
	if err != nil {
		return cookResponse("Please configure properly", err)
	}

	repo := findRepo(reqBody.Environment)
	if !reqBody.Codedeploy {
		return cookResponse("", errors.New("Please enable codedeploy"))
	}

	// start code deployment
	err = awsPackCode(repo, reqBody.Version, reqBody.Artifactid)
	if err == nil {
		err = awsCodeDeploy(*instance, reqBody.Artifactid, reqBody.Version)
	}
	statusMessage := "App successfully deployed to " + reqBody.Environment + ". version ==> " + reqBody.Version
	return cookResponse(statusMessage, err)

}

func awsPackCode(repo, version, artifactID string) error {
	log.Println("moving codedeploy to tmp folder...")
	srcLoc := "./codedeploy"
	destLoc := "/tmp/codedeploy"
	os.RemoveAll(destLoc)
	err := copy.Copy(srcLoc, destLoc)
	if err != nil {
		return err
	}
	log.Println("codedeploy folder successfully moved to /tmp")

	fileURL := nexusBaseURLWithPwd + "?r=" + repo + "&g=" + groupName + "&a=" + artifactID + "&v=" + version + "&p=war"
	log.Println("started downloading war from url ", fileURL)
	if err := downloadFile(javaAppSourcePath, fileURL); err != nil {
		log.Println("Failed while downloading war from nexus ", err)
		return err
	}
	return nil
}

func awsCodeDeploy(instance InstanceConfig, artifactID, version string) error {
	revision := artifactID + "-" + version
	fileName := revision + ".zip"

	tmpDir, err := ioutil.TempDir("/tmp", "app_zip")
	if err != nil {
		return err
	}
	defer func() {
		log.Println("tmpDir cleaned")
		_ = os.RemoveAll(tmpDir)
	}()

	outFilePath, err := zipFile(tmpDir, fileName)
	if err != nil { // error while zipping
		return err
	}
	err = uploadFile(instance, outFilePath, fileName)
	if err != nil { // error while zipping
		return err
	}

	err = registerRevision(instance, fileName, "register revision "+revision)
	if err != nil { // error while zipping
		return err
	}

	return deployRevision(instance, fileName, "deploy revision "+revision)
}

func downloadFile(filepath string, url string) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func zipFile(tmpDir, filename string) (string, error) {
	log.Println("zipping file... ", filename)
	outFilePath := filepath.Join(tmpDir, filename)
	progress := func(archivePath string) {
		fmt.Println(archivePath)
	}
	err := zip.ArchiveFile(javaAppPath, outFilePath, progress)
	return outFilePath, err
}

func uploadFile(instance InstanceConfig, outFilePath, filename string) error {
	log.Println("uploading file from ... ", outFilePath)
	file, err := os.Open(outFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uploader := s3manager.NewUploader(getAwsSession(instance))

	filename = instance.Product.S3Folder + filename
	_, err = uploader.Upload(&s3manager.UploadInput{
		Body:   file,
		Bucket: aws.String(instance.BucketName),
		Key:    aws.String(filename),
	})

	if err != nil {
		log.Println("File upload failed for file ", filename, err)
	}
	return err
}

func registerRevision(instance InstanceConfig, fileName, desc string) error {
	client := codedeploy.New(getAwsSession(instance))
	req, resp := client.RegisterApplicationRevisionRequest(createApplicationRevision(instance, fileName, desc))

	err := req.Send()
	if err == nil { // resp is now filled
		log.Println("new revision registerd ", resp)
	}
	return err
}

func deployRevision(instance InstanceConfig, fileName, desc string) error {
	client := codedeploy.New(getAwsSession(instance))
	req, resp := client.CreateDeploymentRequest(createDeploymentRequest(instance, fileName, desc))

	err := req.Send()
	if err == nil { // resp is now filled
		log.Println("new revision deployment started ", resp)
	}
	return err
}

func reqBodyDecode(body string) (*RequestBody, error) {
	var t *RequestBody
	err := json.Unmarshal([]byte(body), &t)
	return t, err
}

func getAwsSession(instance InstanceConfig) *session.Session {
	if awsSession != nil {
		return awsSession
	}
	// Incase if you are using different vpc use this
	//awsSession = session.New(&aws.Config{Region: aws.String(s3Region), Credentials: credentials.NewStaticCredentials(instance.AwsAccessKey, instance.AwsSecretKey, "")})
	awsSession = session.New(&aws.Config{Region: aws.String(s3Region)})
	return awsSession
}

func createApplicationRevision(instance InstanceConfig, fileName, desc string) *codedeploy.RegisterApplicationRevisionInput {
	registerApplicationRevisionInput := &codedeploy.RegisterApplicationRevisionInput{
		ApplicationName: aws.String(instance.Product.DeploymentApp),
		Description:     aws.String(desc),
		Revision:        revisionLocation(instance, fileName),
	}
	return registerApplicationRevisionInput
}

func createDeploymentRequest(instance InstanceConfig, fileName, desc string) *codedeploy.CreateDeploymentInput {
	createDeploymentInput := &codedeploy.CreateDeploymentInput{
		ApplicationName:     aws.String(instance.Product.DeploymentApp),
		Description:         aws.String(desc),
		Revision:            revisionLocation(instance, fileName),
		DeploymentGroupName: aws.String(instance.Product.DeploymentGroup),
	}
	return createDeploymentInput
}

func revisionLocation(instance InstanceConfig, fileName string) *codedeploy.RevisionLocation {
	fileName = instance.Product.S3Folder + fileName
	s3Location := &codedeploy.S3Location{
		Bucket:     aws.String(instance.BucketName),
		Key:        aws.String(fileName),
		BundleType: aws.String(codedeploy.BundleTypeZip),
	}
	revisionLocation := &codedeploy.RevisionLocation{
		RevisionType: aws.String(codedeploy.RevisionLocationTypeS3),
		S3Location:   s3Location,
	}
	return revisionLocation
}

func findRepo(env string) string {
	switch env {
	case "qa", "staging":
		return "snapshots"
	case "prod":
		return "releases"
	default:
		return "snapshots"
	}
}

func configProductDynamically(env, productStr string) *Product {
	return &Product{
		DeploymentApp:   fmt.Sprintf("%v-app-%v", productStr, env),
		DeploymentGroup: fmt.Sprintf("%v-group-%v", productStr, env),
		S3Folder:        fmt.Sprintf("%v-folder-%v/", productStr, env),
	}
}

func configInstance(env, productStr string) (*InstanceConfig, error) {
	instance := viper.New()
	switch env {
	case "prod":
		instance.SetConfigName("prod")
	default:
		instance.SetConfigName("staging")
	}

	instance.AddConfigPath("./environment")
	err := instance.ReadInConfig()
	if err != nil {
		return nil, err
	}
	instanceConfig := &InstanceConfig{}
	instanceConfig.AwsAccessKey = instance.GetString("aws.access_key")
	instanceConfig.AwsSecretKey = instance.GetString("aws.secret_key")
	instanceConfig.BucketName = instance.GetString("aws.bucket_name")
	instanceConfig.Product = configProductDynamically(env, productStr)
	log.Println("make sure to create an code deploy application ", instanceConfig.Product.DeploymentApp)
	log.Println("make sure to create an code deploy group ", instanceConfig.Product.DeploymentGroup)
	log.Println("make sure to create an s3 folder ", instanceConfig.Product.S3Folder)
	log.Println("make sure to create an bucket ", instanceConfig.BucketName)

	if instanceConfig.Product == nil {
		return nil, errors.New("product not set")
	}
	log.Println("preparing deployment for environment:: ", env, " and for product ::", productStr)

	return instanceConfig, err
}

//Product used internally for setting product based variables
type Product struct {
	DeploymentApp   string
	DeploymentGroup string
	S3Folder        string
}

//InstanceConfig used internally for setting environment based variables
type InstanceConfig struct {
	AwsAccessKey string
	AwsSecretKey string
	BucketName   string
	PemFileLoc   string
	Product      *Product
}

func cookResponse(statusMessage string, err error) (events.APIGatewayProxyResponse, error) {
	statusCode := 200
	if err != nil {
		log.Println("error2:: ", err)
		statusCode = 500
		statusMessage = "error :: " + err.Error()
	}
	return events.APIGatewayProxyResponse{
		Body:       statusMessage,
		StatusCode: statusCode,
	}, err
}
