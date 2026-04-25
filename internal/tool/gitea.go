package tool

import (
	"context"
	"encoding/json"

	"code.gitea.io/sdk/gitea"
	"github.com/tmc/langchaingo/tools"
)

// ---------- query input struct ----------

// GiteaQuery is the input parameter struct that LLM agents must JSON-encode
// when calling any of the Gitea tools. Not all fields are used by every tool;
// unused fields are silently ignored.
type GiteaQuery struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort"`  // only "created" | "update" | "size" | "id"
	Order    string `json:"order"` // "asc" or "desc"
	OrgName  string `json:"org_name"`
	RepoName string `json:"repo_name"`
	Ref      string `json:"ref"` // branch, tag, or commit SHA
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
}

// ---------- List Orgs ----------

// GiteaListOrgsTool allows an LLM agent to list the authenticated user's
// organizations.
type GiteaListOrgsTool struct {
	client *gitea.Client
}

func (t *GiteaListOrgsTool) Name() string {
	return "Gitea List Orgs Tool"
}

func (t *GiteaListOrgsTool) Description() string {
	return `Gitea tool: list all organizations accessible to the authenticated user.

Input JSON fields:
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"page": 1, "page_size": 50}

Output: JSON array of organizations.

Use this tool to discover which Gitea organizations the current user belongs to.`
}

func (t *GiteaListOrgsTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.ListMyOrgs(gitea.ListOrgsOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- List Org Repos ----------

// GiteaListOrgReposTool allows an LLM agent to list repositories in a given
// organization.
type GiteaListOrgReposTool struct {
	client *gitea.Client
}

func (t *GiteaListOrgReposTool) Name() string {
	return "Gitea List Org Repos Tool"
}

func (t *GiteaListOrgReposTool) Description() string {
	return `Gitea tool: list all repositories in a Gitea organization.

Input JSON fields:
- "org_name" (string, required): Organization name
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"org_name": "my-org", "page": 1, "page_size": 50}

Output: JSON array of repositories.

Use this tool to discover repositories belonging to a specific organization.`
}

func (t *GiteaListOrgReposTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.ListOrgRepos(q.OrgName, gitea.ListOrgReposOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- List Repo Branches ----------

// GiteaListRepoBranchesTool allows an LLM agent to list branches in a given
// repository.
type GiteaListRepoBranchesTool struct {
	client *gitea.Client
}

func (t *GiteaListRepoBranchesTool) Name() string {
	return "Gitea List Repo Branches Tool"
}

func (t *GiteaListRepoBranchesTool) Description() string {
	return `Gitea tool: list all branches in a repository.

Input JSON fields:
- "org_name" (string, required): Organization name
- "repo_name" (string, required): Repository name
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"org_name": "my-org", "repo_name": "my-repo", "page": 1, "page_size": 50}

Output: JSON array of branches.

Use this tool to list available branches in a repository.`
}

func (t *GiteaListRepoBranchesTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.ListRepoBranches(q.OrgName, q.RepoName, gitea.ListRepoBranchesOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Search Repos ----------

// GiteaSearchReposTool allows an LLM agent to search for repositories across
// Gitea.
type GiteaSearchReposTool struct {
	client *gitea.Client
}

func (t *GiteaSearchReposTool) Name() string {
	return "Gitea Search Repos Tool"
}

func (t *GiteaSearchReposTool) Description() string {
	return `Gitea tool: search for repositories by keyword with sorting and ordering.

Input JSON fields:
- "repo_name" (string, optional): Search keyword for repository name
- "sort" (string, optional): Sort field — "created", "update", "size", or "id"
- "order" (string, optional): Sort order — "asc" or "desc"
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"repo_name": "kdiag", "sort": "update", "order": "desc", "page": 1, "page_size": 20}

Output: JSON array of matching repositories.

Use this tool to find repositories across Gitea by name keyword.`
}

func (t *GiteaSearchReposTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.SearchRepos(gitea.SearchRepoOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
		Keyword: q.RepoName,
		Sort:    q.Sort,
		Order:   q.Order,
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Get Tree ----------

// GiteaGetTreeTool allows an LLM agent to retrieve the file/directory tree of
// a repository at a given reference.
type GiteaGetTreeTool struct {
	client *gitea.Client
}

func (t *GiteaGetTreeTool) Name() string {
	return "Gitea Get Tree Tool"
}

func (t *GiteaGetTreeTool) Description() string {
	return `Gitea tool: get the file/directory tree listing of a repository at a given reference.

Input JSON fields:
- "org_name" (string, required): Organization name
- "repo_name" (string, required): Repository name
- "ref" (string, required): Branch, tag, or commit SHA
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"org_name": "my-org", "repo_name": "my-repo", "ref": "main", "page": 1, "page_size": 50}

Output: JSON object with tree entries (recursive).

Use this tool to explore the file structure of a repository at a specific branch or commit.`
}

func (t *GiteaGetTreeTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.GetTrees(q.OrgName, q.RepoName, gitea.ListTreeOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
		Ref:       q.Ref,
		Recursive: true,
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Get Raw File ----------

// GiteaGetRawFileTool allows an LLM agent to retrieve the raw content of a
// file from a repository (non-plain-text files are not suitable).
type GiteaGetRawFileTool struct {
	client *gitea.Client
}

func (t *GiteaGetRawFileTool) Name() string {
	return "Gitea Get Raw File Tool"
}

func (t *GiteaGetRawFileTool) Description() string {
	return `Gitea tool: retrieve the raw content of a file from a repository.
NOTE: only use this for plain-text files; binary files will produce garbled output.

Input JSON fields:
- "org_name" (string, required): Organization name
- "repo_name" (string, required): Repository name
- "file_path" (string, required): Path to the file within the repository
- "ref" (string, required): Branch, tag, or commit SHA

Example input: {"org_name": "my-org", "repo_name": "my-repo", "file_path": "README.md", "ref": "main"}

Output: Raw file content as a string.

Use this tool to read the contents of a specific file in a repository.`
}

func (t *GiteaGetRawFileTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	data, _, err := t.client.GetRawFile(q.OrgName, q.RepoName, q.FilePath, q.Ref)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- List Repo Commits ----------

// GiteaListRepoCommitsTool allows an LLM agent to list commits in a repository,
// optionally filtered by path and starting SHA.
type GiteaListRepoCommitsTool struct {
	client *gitea.Client
}

func (t *GiteaListRepoCommitsTool) Name() string {
	return "Gitea List Repo Commits Tool"
}

func (t *GiteaListRepoCommitsTool) Description() string {
	return `Gitea tool: list commits in a repository, optionally filtered by path and starting SHA.

Input JSON fields:
- "org_name" (string, required): Organization name
- "repo_name" (string, required): Repository name
- "ref" (string, optional): SHA or branch to start listing commits from (e.g., "master")
- "path" (string, optional): Filter commits touching this file path
- "page" (int, optional): Page number for pagination
- "page_size" (int, optional): Number of items per page

Example input: {"org_name": "my-org", "repo_name": "my-repo", "ref": "master", "path": "README.md", "page": 1, "page_size": 30}

Output: JSON array of commits.

Use this tool to browse the commit history of a repository.`
}

func (t *GiteaListRepoCommitsTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	result, _, err := t.client.ListRepoCommits(q.OrgName, q.RepoName, gitea.ListCommitOptions{
		ListOptions: gitea.ListOptions{
			Page:     q.Page,
			PageSize: q.PageSize,
		},
		Path: q.Path,
		SHA:  q.Ref,
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Get Commit Diff ----------

// GiteaGetCommitDiffTool allows an LLM agent to retrieve the diff for a
// specific commit.
type GiteaGetCommitDiffTool struct {
	client *gitea.Client
}

func (t *GiteaGetCommitDiffTool) Name() string {
	return "Gitea Get Commit Diff Tool"
}

func (t *GiteaGetCommitDiffTool) Description() string {
	return `Gitea tool: retrieve the diff of a specific commit.

Input JSON fields:
- "org_name" (string, required): Organization name
- "repo_name" (string, required): Repository name
- "ref" (string, required): Commit SHA

Example input: {"org_name": "my-org", "repo_name": "my-repo", "ref": "abc123def456"}

Output: Raw diff text of the commit.

Use this tool to inspect the changes introduced by a specific commit.`
}

func (t *GiteaGetCommitDiffTool) Call(ctx context.Context, input string) (string, error) {
	var q GiteaQuery
	err := json.Unmarshal([]byte(input), &q)
	if err != nil {
		return "", err
	}
	data, _, err := t.client.GetCommitDiff(q.OrgName, q.RepoName, q.Ref)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- factory ----------

// NewGiteaQueryTool creates and returns the set of Gitea tools that will be
// registered with the LLM agent.
func NewGiteaQueryTool(client *gitea.Client) []tools.Tool {
	return []tools.Tool{
		&GiteaListOrgsTool{client: client},
		&GiteaListOrgReposTool{client: client},
		&GiteaListRepoBranchesTool{client: client},
		&GiteaSearchReposTool{client: client},
		&GiteaGetTreeTool{client: client},
		&GiteaGetRawFileTool{client: client},
		&GiteaListRepoCommitsTool{client: client},
		&GiteaGetCommitDiffTool{client: client},
	}
}
