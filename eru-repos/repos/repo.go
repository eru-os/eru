package repos

import (
	"context"
	"encoding/json"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"time"
)

type RepoI interface {
	Commit(ctx context.Context, repoBytes []byte, repoFileName string) (err error)
	GetAttribute(attrName string) (attrValue interface{})
	GetBranch(ctx context.Context) (branch interface{}, err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	SetAuthKey(ctx context.Context, authKey string)
	SetLastCommitAt()
}

func (repo *Repo) SetLastCommitAt() {
	repo.LastCommitAt = time.Now().Format(time.RFC1123)
}

func (repo *Repo) SetAuthKey(ctx context.Context, authKey string) {
	repo.AuthKey = authKey
	return
}

func (repo *Repo) Commit(ctx context.Context, repoBytes []byte, repoFileName string) (err error) {
	logs.WithContext(ctx).Info("commit method not implemented")
	return
}

func (repo *Repo) GetBranch(ctx context.Context) (branch interface{}, err error) {
	logs.WithContext(ctx).Info("get_branch method not implemented")
	return
}

func (repo *Repo) GetAttribute(attrName string) (attrValue interface{}) {
	switch attrName {
	case "branch_name":
		return repo.BranchName
	case "repo_name":
		return repo.RepoName
	case "repo_type":
		return repo.RepoType
	case "auth_key":
		return repo.AuthKey
	case "auto_commit":
		return repo.AutoCommit
	case "auth_mode":
		return repo.AuthMode
	default:
		return nil
	}
}

type Repo struct {
	RepoType       string `json:"repo_type"`
	RepoName       string `json:"repo_name"`
	BranchName     string `json:"branch_name"`
	AuthMode       string `json:"auth_mode"`
	AuthKey        string `json:"auth_key"`
	AutoCommit     bool   `json:"auto_commit"`
	InstallationId string `json:"installation_id"`
	LastCommitAt   string `json:"last_commit_at"`
}

type RepoToken struct {
	RepoToken       string `json:"repo_token"`
	RepoTokenExpiry string `json:"repo_token_expiry"`
}

func GetRepo(repoType string) RepoI {
	switch repoType {
	case "GITHUB":
		gr := new(GithubRepo)
		//gr.BranchName = repo.GetAttribute("branchName").(string)
		//gr.RepoName = repo.GetAttribute("repoName").(string)
		//gr.RepoType = repo.GetAttribute("repoType").(string)
		//gr.AuthKey = repo.GetAttribute("authKey").(string)
		//gr.AuthMode = repo.GetAttribute("authMode").(string)
		//gr.AutoCommit = repo.GetAttribute("autoCommit").(bool)
		return gr
	default:
		return new(Repo)
	}
}

func (repo *Repo) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &repo)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
