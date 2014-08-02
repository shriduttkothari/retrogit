package githop

import (
	"sort"
	"time"

	"github.com/google/go-github/github"
)

type RepoDigest struct {
	Repo    *github.Repository
	Commits []github.RepositoryCommit
}

// sort.Interface implementation for sorting RepoDigests.
type ByRepoFullName []*RepoDigest

func (a ByRepoFullName) Len() int           { return len(a) }
func (a ByRepoFullName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRepoFullName) Less(i, j int) bool { return *a[i].Repo.FullName < *a[j].Repo.FullName }

type Digest struct {
	User        *github.User
	StartTime   time.Time
	EndTime     time.Time
	RepoDigests []*RepoDigest
}

func newDigest(githubClient *github.Client) (*Digest, error) {
	user, _, err := githubClient.Users.Get("")
	if err != nil {
		return nil, err
	}

	// The username parameter must be left blank so that we can get all of the
	// repositories the user has access to, not just ones that they own.
	repos, _, err := githubClient.Repositories.List("", nil)
	if err != nil {
		return nil, err
	}

	orgs, _, err := githubClient.Organizations.List("", nil)
	if err != nil {
		return nil, err
	}
	for _, org := range orgs {
		orgRepos, _, err := githubClient.Repositories.ListByOrg(*org.Login, nil)
		if err != nil {
			return nil, err
		}
		newRepos := make([]github.Repository, len(repos)+len(orgRepos))
		copy(newRepos, repos)
		copy(newRepos[len(repos):], orgRepos)
		repos = newRepos
	}

	now := time.Now()
	digestStartTime := time.Date(now.Year()-1, now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	digestEndTime := digestStartTime.AddDate(0, 0, 1)

	// Only look at repos that may have activity in the digest interval.
	var digestRepos []github.Repository
	for _, repo := range repos {
		if repo.CreatedAt.Before(digestEndTime) && repo.PushedAt.After(digestStartTime) {
			digestRepos = append(digestRepos, repo)
		}
	}
	repos = digestRepos
	digest := &Digest{
		User:        user,
		RepoDigests: make([]*RepoDigest, 0, len(repos)),
		StartTime:   digestStartTime,
		EndTime:     digestEndTime,
	}
	err = digest.fetch(repos, githubClient)
	return digest, err
}

func (digest *Digest) fetch(repos []github.Repository, githubClient *github.Client) error {
	type RepoDigestResponse struct {
		repoDigest *RepoDigest
		err        error
	}
	ch := make(chan *RepoDigestResponse)
	for _, repo := range repos {
		go func(repo github.Repository) {
			commits, _, err := githubClient.Repositories.ListCommits(
				*repo.Owner.Login,
				*repo.Name,
				&github.CommitsListOptions{
					Author: *digest.User.Login,
					Since:  digest.StartTime,
					Until:  digest.EndTime,
				})
			if err != nil {
				ch <- &RepoDigestResponse{nil, err}
			} else {
				ch <- &RepoDigestResponse{&RepoDigest{&repo, commits}, nil}
			}
		}(repo)
	}
	for i := 0; i < len(repos); i++ {
		select {
		case r := <-ch:
			if r.err != nil {
				return r.err
			}
			digest.RepoDigests = append(digest.RepoDigests, r.repoDigest)
		}
	}
	sort.Sort(ByRepoFullName(digest.RepoDigests))
	return nil
}