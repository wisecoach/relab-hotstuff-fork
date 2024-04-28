#!/bin/bash

join () {
	local IFS="$1"
	shift
	echo "$*"
}

replicas=${1-4}
leader_rotation=${2-reputation}
batch_size=${3-200}
max_concurrent=${4-200}
rate_limit=${5-100000}
target_file=${6-"test_base/$(date +%s)_test-log"}
payload_size=${7-"10"}
new_epoch_rate=${8-"0"}
new_epoch_duration=${9-"2s"}
byzantine=${10-""}

compose_args="--project-name=robust-hotstuff"

num_hosts=18

declare -A hosts

for ((i=1; i<=num_hosts; i++)); do
	hosts[$i]="hotstuff_worker_$i"
done

echo $(join ' ' "${hosts[@]}")


if [ ! -f "./id" ]; then
	ssh-keygen -t ed25519 -C "hotstuff-test" -f "./id" -N ""
fi

docker-compose $compose_args down
docker-compose $compose_args up -d

sleep 1

mkdir -p $target_file

docker-compose $compose_args exec -T controller /bin/sh -c "ssh-keyscan -p 10033 -H $(join ' ' "${hosts[@]}") >> ~/.ssh/known_hosts" &>/dev/null
docker-compose $compose_args exec -T controller /bin/sh -c "hotstuff run --hosts '$(join ',' "${hosts[@]}")' --replicas ${replicas} --config ./example_config.toml --log-level debug --output ./test-log --leader-rotation ${leader_rotation} --batch-size ${batch_size} --max-concurrent ${max_concurrent} --rate-limit ${rate_limit} ${byzantine} --payload-size ${payload_size} --new-epoch-rate ${new_epoch_rate} --new-epoch-duration ${new_epoch_duration}" 2> $target_file/test.log
exit_code="$?"

docker cp hotstuff_controller:/root/test-log $target_file
./handle_data.py $target_file/test-log > $target_file/performance.txt

exit $exit_code
