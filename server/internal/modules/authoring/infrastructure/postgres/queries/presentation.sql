-- name: CreatePublishedFile :exec
INSERT INTO problem_version_files(problem_id,version_no,file_id,path,filename,media_type,purpose,blob_sha256,byte_size,sample_index,preview,truncated,is_binary,embedded)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14);
