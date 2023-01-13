package funcs

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/yzbtdiy/alist_batch/models"

	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	"gopkg.in/yaml.v3"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func CheckFile(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return true
}

func GetConfig(fileName string) *models.Config {
	// _conf := models.Config{}
	var _conf *models.Config
	content, err := os.ReadFile("./" + fileName)
	if err != nil {
		fmt.Println("读取配置文件出错")
		fmt.Println(err)
	}
	err = yaml.Unmarshal(content, &_conf)
	if err != nil {
		fmt.Println(err)
	}
	return _conf
}

// 修改配置文件, 添加token
func ModConfig(fileName string, oldConf *models.Config, token string) {
	oldConf.Token = token
	newConf, err := yaml.Marshal(oldConf)
	if err != nil {
		fmt.Println(err)
	}
	os.WriteFile("./"+fileName, newConf, 0o777)
}

func GetShareList(fileName string) map[string]map[string]string {
	shareListContent := make(map[string]map[string]string)
	content, err := os.ReadFile("./" + fileName)
	if err != nil {
		fmt.Println("读取阿里分享列表文件出错")
		fmt.Println(err)
	}
	yaml.Unmarshal(content, &shareListContent)
	return shareListContent
}

func BuildPushData(mountPath string, aliUrl string, config *models.Config) string {
	reId, _ := regexp.Compile("https://www.aliyundrive.com/s/(.*)/folder")
	reFolder, _ := regexp.Compile("/folder/(.*)$")
	reShareId := reId.FindStringSubmatch(aliUrl)
	reShareFolder := reFolder.FindStringSubmatch(aliUrl)
	shareId := reShareId[len(reShareId)-1]
	shareFolder := reShareFolder[len(reShareFolder)-1]

	addition := new(models.Addition)
	addition.RefreshToken = config.RefreshToken
	addition.ShareId = shareId
	addition.SharePwd = ""
	addition.RootFolderId = shareFolder
	addition.OrderBy = ""
	addition.OrderDirection = ""

	additionJson, _ := json.Marshal(addition)
	additionData := string(additionJson)

	data := models.PushData{
		MountPath:       mountPath,
		Order:           0,
		Remark:          "",
		CacheExpiration: 30,
		WebProxy:        false,
		WebdavPolicy:    "302_redirect",
		DownProxyUrl:    "",
		ExtractFolder:   "",
		Driver:          "AliyundriveShare",
		Addition:        additionData,
	}
	pushJson, _ := json.Marshal(data)
	pushData := string(pushJson)
	return pushData
}

func Start() {
	// 读取配置文件
	conf := GetConfig("config.yaml")

	if conf.Url == "ALIST_URL" {
		fmt.Println("url 未配置, 请检查配置文件")
	}
	if conf.RefreshToken == "ALI_YUNPAN_REFRESH_TOKEN" {
		fmt.Println("refresh_token 未配置, 请检查配置文件")
	}

	if conf.Auth.Username == "USERNAME" && conf.Auth.Password == "PASSWORD" && conf.Token == "ALIST_TOKEN" {
		fmt.Println("token和用户密码至少要配置一项, 请检查配置文件")
	}

	// 拼接API
	loginApi := conf.Url + "/api/auth/login"
	storageListApi := conf.Url + "/api/admin/storage/list"

	// 将用户名和密码转为json
	loginData := models.AuthJson{
		Username: conf.Auth.Username,
		Password: conf.Auth.Password,
	}
	authJson, _ := json.Marshal(loginData)

	// 用户登录信息拼接字符串
	// loginData := `{"username":"` + conf.Auth.Username + `","password":"` + conf.Auth.Password + `"}`
	// fmt.Println(loginData)

	// 创建http client
	httpClient := resty.New()
	// 通过尝试存储列表验证token是否有效
	if conf.Token != "ALIST_TOKEN" && conf.Token != "" {
		resData := &models.ResData{}
		httpClient.R().SetResult(resData).
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", conf.Token).
			Get(storageListApi)
		if resData.Code == 200 {
			shareListData := GetShareList("./ali_share.yaml")
			for category, shareList := range shareListData {
				// fmt.Println(category)
				for shareName, shareUrl := range shareList {
					// fmt.Println(shareName, shareUrl)
					for {
						if do(httpClient, conf, category, shareName, shareUrl) {
							fmt.Println(category + " " + shareName + " 添加完成")
							break
						}
						time.Sleep(3 * time.Second)
					}
				}
			}
		}
	} else {
		// token无效重新获取
		fmt.Println("token无效, 尝试重新获取")
		loginResData := &models.LoginRes{}
		httpClient.R().SetResult(loginResData).
			SetHeader("Content-Type", "application/json").
			SetBody(string(authJson)).
			Post(loginApi)
		if loginResData.Code == 200 {
			ModConfig("config.yaml", conf, loginResData.Data.Token)
			fmt.Println("token已更新, 请重新运行此程序")
		}
	}
}

func do(httpClient *resty.Client, conf *models.Config, category, shareName, shareUrl string) bool {
	pushData := BuildPushData(`/`+category+`/`+shareName, shareUrl, conf)
	resData := &models.ResData{}
	httpClient.R().SetResult(resData).
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", conf.Token).
		SetBody(pushData).
		Post(conf.Url + "/api/admin/storage/create")
	if resData.Code != 200 {
		if resData.Message != `failed create storage in database: ERROR: duplicate key value violates unique constraint "x_storages_mount_path_key" (SQLSTATE 23505)` {
			fmt.Println(category + " " + shareName + " 添加失败, 请检查是否重复添加")
			return false
		}
	}
	return true
}
