load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")

# gazelle:prefix github.com/chascale/chascale-server
gazelle(name = "gazelle")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/chascale/chascale-server",
    visibility = ["//visibility:private"],
    deps = [
        "//server:go_default_library",
        "@com_github_gorilla_websocket//:go_default_library",
    ],
)

go_binary(
    name = "chascale-server",
    embed = [":go_default_library"],
    goarch = "amd64",
    goos = "linux",
    pure = "on",
    visibility = ["//visibility:public"],
)

go_image(
    name = "chascale-server-container",
    binary = ":chascale-server",
    ports = ["8080"],
)
