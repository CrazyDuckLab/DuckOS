# Beta.4 source snapshot and historical disclosure

The release snapshot removes eight tracked `.runtime-data` laboratory reports.
`.gitignore` excludes their directory from ordinary additions; `.gitattributes`
sets export-ignore as defense in depth for source archives. Logs, backups and
credentials are not release assets. Ignore rules can be bypassed with force-add;
release inventory validation must also reject runtime evidence.

This is a normal descendant commit. History and existing tags are preserved;
removal from HEAD does not erase old blobs or previously downloaded copies.
Historical management addresses and diagnostic topology remain reachable, and
at least one old blob was confirmed available through GitHub API. The address
finding alone does not establish an active token/private-key compromise or a
publicly reachable management endpoint. No live endpoint probing is performed.

The minimal publication plan is a new dedicated release branch/ref pointing at
the final source and a new annotated release tag, after explicit owner approval.
Existing releases/tags and unrelated development branches remain unchanged. The owner has authorized publication with the known historical topology
disclosure and existing history preserved. This does not authorize publishing
active credentials or newly discovered material security findings.
