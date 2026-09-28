package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
	"net/http"
)

const baseUrl = "https://api.github.com"

type GithubRepo struct {
	Repo
}

func (githubRepo *GithubRepo) Commit(ctx context.Context, repoBytes []byte, repoFileName string) (err error) {
	logs.WithContext(ctx).Info("Commit called from GithubRepo")
	res, err := githubRepo.GetBranch(ctx)
	if err != nil {
		return
	}
	branch_sha := ""
	branch_commit_sha := ""
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		if commitObj, commitObjOk := resMap["commit"]; commitObjOk {
			if commitMap, commitMapOk := commitObj.(map[string]interface{}); commitMapOk {
				branch_sha = commitMap["sha"].(string)
				if innerCommitObj, innerCommitObjOk := commitMap["commit"]; innerCommitObjOk {
					if innerCommitMap, innerCommitMapOk := innerCommitObj.(map[string]interface{}); innerCommitMapOk {
						if treeObj, treeObjOk := innerCommitMap["tree"]; treeObjOk {
							if treeMap, treeMapOk := treeObj.(map[string]interface{}); treeMapOk {
								branch_commit_sha = treeMap["sha"].(string)
							} else {
								err = errors.New("GetBranch response tree object is not a map")
							}
						} else {
							err = errors.New("GetBranch response inner commit object does not have tree object")
						}
					} else {
						err = errors.New("GetBranch response inner commit object is not a map")
					}
				} else {
					err = errors.New("GetBranch response commit object does not have inner commit object")
				}
			} else {
				err = errors.New("GetBranch response commit object is not a map")
			}
		} else {
			err = errors.New("GetBranch response does not have a commit object")
		}
	} else {
		err = errors.New("GetBranch response body is not a map")
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	res, err = githubRepo.GetTree(ctx, branch_commit_sha)
	if err != nil {
		return
	}
	file_sha := ""
	_ = file_sha
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		if treeObj, treeObjOk := resMap["tree"]; treeObjOk {
			if treeMap, treeMapOk := treeObj.([]interface{}); treeMapOk {
				for _, v := range treeMap {
					if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
						if vMap["path"].(string) == repoFileName {
							file_sha = vMap["sha"].(string)
							break
						}
					} else {
						err = errors.New("GetTree tree object is not a map")
					}
				}
			} else {
				err = errors.New("GetTree tree object is not array")
			}
		} else {
			err = errors.New("GetTree response does not have a tree object")
		}
	} else {
		err = errors.New("GetTree response body is not a map")
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	res, err = githubRepo.CreateTree(ctx, branch_commit_sha, repoFileName, string(repoBytes))
	if err != nil {
		return
	}
	new_tree_sha := ""
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		new_tree_sha = resMap["sha"].(string)
	} else {
		err = errors.New("GetCommit response body is not a map")
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	res, err = githubRepo.CreateCommit(ctx, new_tree_sha, branch_sha)
	if err != nil {
		return
	}
	new_commit_sha := ""
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		new_commit_sha = resMap["sha"].(string)
	} else {
		err = errors.New("CreateCommit response body is not a map")
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	res, err = githubRepo.UpdateRef(ctx, new_commit_sha)
	if err != nil {
		return
	}
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (githubRepo *GithubRepo) GetBranch(ctx context.Context) (branch interface{}, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Bearer ", githubRepo.AuthKey))
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/vnd.github+json")
	headers.Set("X-GitHub-Api-Version", "2022-11-28")
	url := fmt.Sprint(baseUrl, "/repos/", githubRepo.RepoName, "/branches/", githubRepo.BranchName)
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, nil, nil, nil, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return res, err
}

func (githubRepo *GithubRepo) GetTree(ctx context.Context, treeSha string) (tree interface{}, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Bearer ", githubRepo.AuthKey))
	headers.Set("Content-Type", "application/json")
	url := fmt.Sprint(baseUrl, "/repos/", githubRepo.RepoName, "/git/trees/", treeSha)
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, nil, nil, nil, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return res, err
}

func (githubRepo *GithubRepo) CreateTree(ctx context.Context, treeSha string, repoFileName string, content string) (tree interface{}, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Bearer ", githubRepo.AuthKey))
	headers.Set("Content-Type", "application/json")
	url := fmt.Sprint(baseUrl, "/repos/", githubRepo.RepoName, "/git/trees")

	postBody := make(map[string]interface{})
	postBody["base_tree"] = treeSha
	treeBody := make(map[string]interface{})
	treeBody["path"] = repoFileName
	treeBody["mode"] = "100644"
	treeBody["type"] = "blob"
	treeBody["content"] = content
	var treeObjArray []map[string]interface{}
	treeObjArray = append(treeObjArray, treeBody)
	postBody["tree"] = treeObjArray

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, postBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return res, err
}

func (githubRepo *GithubRepo) CreateCommit(ctx context.Context, new_tree_sha string, branch_sha string) (commit interface{}, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Bearer ", githubRepo.AuthKey))
	headers.Set("Content-Type", "application/json")
	url := fmt.Sprint(baseUrl, "/repos/", githubRepo.RepoName, "/git/commits")

	postBody := make(map[string]interface{})
	postBody["message"] = "from eru app"
	postBody["tree"] = new_tree_sha
	var parents []string
	parents = append(parents, branch_sha)
	postBody["parents"] = parents

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, nil, postBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return res, err
}

func (githubRepo *GithubRepo) UpdateRef(ctx context.Context, new_commit_sha string) (ref interface{}, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Bearer ", githubRepo.AuthKey))
	headers.Set("Content-Type", "application/json")
	url := fmt.Sprint(baseUrl, "/repos/", githubRepo.RepoName, "/git/refs/heads/", githubRepo.BranchName)

	postBody := make(map[string]interface{})
	postBody["sha"] = new_commit_sha

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPatch, url, headers, nil, nil, nil, postBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return res, err
}

func (githubRepo *GithubRepo) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &githubRepo)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
