# Contributing to Syver

Thank you for your interest in contributing to Syver. Syver wouldn't be where it is today if it wasn't for people like you.
Some ways you can contribute:

* Improve the [README](https://github.com/krameff/syver/blob/main/README.md)
    and/or [Docs](https://github.com/krameff/syver/tree/main/docs/).
    This makes it easier for new users to learn syver.
* Vote on bugs and feature requests by adding a :+1: reaction to the inital post.
* Create tutorials, blog posts and example use-cases on how to use Syver.
* Help users with [questions](https://github.com/krameff/syver/labels/question) tracker.
* Fix verified [bugs](https://github.com/krameff/syver/issues?q=is%3Aopen+is%3Aissue+label%3Aapproved+label%3Abug+sort%3Areactions-%2B1-desc).
* Implement approved [feature requests](https://github.com/krameff/syver/issues?q=is%3Aopen+is%3Aissue+label%3Aapproved+label%3Aenhancement+sort%3Areactions-%2B1-desc).
* Spread the word.

## Features and bug reports and questions

Please search the [issues](https://github.com/krameff/syver/issues) page before opening a feature request or a bug report.
If a feature or a bug report already exists,
please thumbs up the initial post to indicate it's importance to you and raise it's priority.
Please comment and contribute to said issue if you feel it's deficient.

## Bug reports

If you think you found a bug in Syver, please submit a [bug report](https://github.com/krameff/syver/issues).

## Feature requests

If there's a feature you wish Syver would support, please open a feature request.

Some things to note prior to opening a Syver feature request:

* Syver is intended to be quick and easy to learn.
* Syver is focused on the 20% of the 80/20 rule.
    In other words, Syver focuses on the 20% of features that cover the core aspects of OS testing and benefit 80% of users.
* Syver is intended to test the local machine it's running on.
    Tests aren't intended to be used to validate remote systems or endpoints.
* Syver provides a generic [command](https://github.com/krameff/syver/blob/main/docs/gossfile.md#command) runner
    to allow users to cover more nuanced test cases.

If you believe your feature adheres to the goals of Syver,
please open a [feature request](https://github.com/krameff/syver/issues) on GitHub
which describes the feature you would like to see, why it is useful, and how it should work.

Once a feature is submitted, it will be reviewed.
Upon approval, the issue can be worked on and PRs can be submitted that implement this new feature.

## Contributing code and documentation changes

If you have a bugfix or new feature that you would like to contribute to Syver, please find or open an issue about it first.
Talk about what you would like to do. It may be that somebody is already working on it,
or that there are particular issues that you should know about before implementing the change.

We enjoy working with contributors to get their code accepted.
There are many approaches to fixing a problem and it is important to find the best approach before writing too much code.

Note that it is unlikely the project will merge refactors for the sake of refactoring
or niche features that aren't common use-cases (see the feature request section above).
These types of pull requests have a high cost to maintainers but provide little benefit to the community.

Lastly, in order for a pull request to be merged,
it must provide automated tests (unit and/or integration) proving the change works as intended,
this also prevents future changes from introducing regressions.
It would be quite odd for a testing tool to not have a healthy approach to test automation, after all. :smile:

Before opening a PR, run `make pre-push` locally (formats, vets, lints, and runs unit
tests plus the discovery/depends-on E2E and security checks CI runs). See
[`docs/testing.md`](https://github.com/krameff/syver/blob/main/docs/testing.md) for the full breakdown, including an optional
git hook (`git config core.hooksPath .githooks`) that runs a fast subset on every commit.
