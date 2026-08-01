module.exports = {
  branches: ["main"],
  tagFormat: "v${version}",
  plugins: [
    "@semantic-release/commit-analyzer",
    "@semantic-release/release-notes-generator",
    [
      "@semantic-release/github",
      {
        draftRelease: true,
        successComment: false,
        failComment: false,
      },
    ],
    [
      "@semantic-release/exec",
      {
        successCmd: "node scripts/write-release-output.mjs ${nextRelease.version}",
      },
    ],
  ],
};
