package version

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/Masterminds/semver/v3"
)

// RepoHead provides statistics about the head commit of a git
// repository like its commit-hash, the number of commits since
// the last tag and the name of the last tag.
type RepoHead struct {
	LastTag         string
	CommitsSinceTag int
	Hash            string
}

type options struct {
	matchFunc func(string) bool
}

type Option = func(*options)

func WithMatchPattern(pattern string) Option {
	return func(opts *options) {
		opts.matchFunc = func(tagName string) bool {
			if pattern == "" {
				return true
			}
			matched, err := filepath.Match(pattern, tagName)
			if err != nil {
				fmt.Printf("Ignoring invalid match pattern: %s: %s\n", pattern, err)
				return true
			}
			return matched
		}
	}
}

// GitDescribeRepository takes an existing loaded Git repository instead
// of a local path (for use cases of using in-memory repositories).
func GitDescribeRepository(repo *git.Repository, opts ...Option) (*RepoHead, error) {
	options := options{matchFunc: func(string) bool { return true }}
	for _, apply := range opts {
		apply(&options)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve repo head: %w", err)
	}

	ref := RepoHead{
		Hash: head.Hash().String(),
	}

	tags, err := repo.Tags()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve tags: %w", err)
	}

	var tag string

	var tagMap map[string]string = make(map[string]string)
	if err = tags.ForEach(func(t *plumbing.Reference) error {
		tagName := t.Name().Short()
		if !options.matchFunc(tagName) {
			return nil
		}

		rev, err := repo.ResolveRevision(plumbing.Revision(t.Name()))
		if err != nil {
			return nil
		}

		if *rev == head.Hash() {
			tag = tagName
		}

		if existingTag, ok := tagMap[rev.String()]; ok {
			mapVersion, err := semver.NewVersion(existingTag)
			if err != nil {
				return nil
			}

			tagVersion, err := semver.NewVersion(tagName)
			if err != nil {
				return nil
			}

			if mapVersion.LessThan(tagVersion) {
				tagMap[rev.String()] = tagName
			}
		} else {
			tagMap[rev.String()] = tagName
		}

		return nil
	}); err == storer.ErrStop && tag != "" {
		ref.LastTag = tag
		return &ref, nil
	}

	commits, err := repo.Log(&git.LogOptions{
		From:  head.Hash(),
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	_ = commits.ForEach(func(c *object.Commit) error {
		tag, ok := tagMap[c.Hash.String()]
		if ok {
			ref.LastTag = tag
			return storer.ErrStop
		}
		ref.CommitsSinceTag++
		return nil
	})
	return &ref, nil
}

// GitDescribe looks at the git repository at path and figures
// out versioning relvant information about the head commit.
func GitDescribe(path string, opts ...Option) (*RepoHead, error) {
	openOpts := git.PlainOpenOptions{DetectDotGit: true}
	repo, err := git.PlainOpenWithOptions(path, &openOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	return GitDescribeRepository(repo, opts...)
}
