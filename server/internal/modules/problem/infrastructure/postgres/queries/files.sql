-- name: ListPublishedFiles :many
SELECT f.file_id,f.path,f.filename,f.media_type,f.purpose,f.blob_sha256,f.byte_size,f.sample_index,f.preview,f.truncated,f.is_binary,f.embedded
FROM problem_version_files f JOIN problems p ON p.id=f.problem_id
WHERE p.domain_id=sqlc.arg(domain_id) AND f.problem_id=sqlc.arg(problem_id) AND f.version_no=sqlc.arg(version_no)
ORDER BY f.sample_index,f.purpose,f.file_id;

-- name: GetPublishedFile :one
SELECT f.file_id,f.path,f.filename,f.media_type,f.purpose,f.blob_sha256,f.byte_size,f.sample_index,f.preview,f.truncated,f.is_binary,f.embedded
FROM problem_version_files f JOIN problems p ON p.id=f.problem_id
WHERE p.domain_id=sqlc.arg(domain_id) AND f.problem_id=sqlc.arg(problem_id) AND f.version_no=sqlc.arg(version_no) AND f.file_id=sqlc.arg(file_id);
