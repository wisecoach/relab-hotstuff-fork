#!/home/wisecoach/anaconda3/bin/python
import os
import pandas as pd
import matplotlib.pyplot as plt
import argparse
import re
import time
from datetime import datetime

leader_rotation_list = ["reputation", "round-robin"]
consensus_list = ["chainedhotstuff", "fasthotstuff"]
client_num = 1
os.system("./rebuild.sh")
os.system("docker build -t hotstuff_controller -f /home/wisecoach/go/src/relab-hotstuff-fork/scripts/Dockerfile.controller .")


def base():
    df = pd.read_csv("test_base_examples.csv")
    for leader_rotation in leader_rotation_list:
        for index, row in df.iterrows():
            batch_size = row["batch_size"]
            max_concurrent = row["max_concurrent"]
            rate_limit = row["rate_limit"]
            payload_size = row["payload_size"]
            print(f"start test: batch_size: {batch_size}, rate_limit: {rate_limit}")
            replicas = 4
            target_file = f"test_base/test_base_{index}_{replicas}_{leader_rotation}_{batch_size}_{rate_limit}_{payload_size}"
            if not os.path.exists(target_file):
                cmd = f"./test_base.sh {replicas} {leader_rotation} {batch_size} {max_concurrent} {int(rate_limit/client_num)} {target_file} {payload_size}> /dev/null 2>&1"
                os.system(cmd)
            else:
                print(f"target file {target_file} already exists, skip the example")


def resolve_data(test_dir):
    example_data = []
    for example in os.listdir(test_dir):
        args = example.split(test_dir + "_")[1].split("_")
        index = args[0]
        replicas = args[1]
        leader_rotation = args[2]
        batch_size = args[3]
        rate_limit = args[4]
        payload_size = int(args[5]) / client_num
        if not os.path.exists(f"{test_dir}/{example}/performance.txt"):
            return example
        with open(f"{test_dir}/{example}/performance.txt", 'r') as fp:
            lines = fp.readlines()
            if len(lines) < 3:
                return example
            block_throughput = float(lines[0].split(":")[-1])
            throughput = float(lines[1].split(":")[-1])
            latency = float(lines[2].split(":")[-1])
            if block_throughput == 0:
                return example
            example_data.append({
                "index": index,
                "replicas": replicas,
                "leader_rotation": leader_rotation,
                "batch_size": batch_size,
                "rate_limit": rate_limit,
                "payload_size": payload_size,
                leader_rotation + ":block_throughput": block_throughput,
                leader_rotation + ":throughput": throughput,
                leader_rotation + ":latency": latency
            })
    example_data.sort(key=lambda x: (x['leader_rotation'], int(x["index"])))
    max_index = max([int(example["index"]) for example in example_data])
    for index, example in enumerate(example_data):
        if index != int(example["index"]):
            example_data[int(example["index"])][example["leader_rotation"] + ":block_throughput"] = example[example["leader_rotation"] + ":block_throughput"]
            example_data[int(example["index"])][example["leader_rotation"] + ":throughput"] = example[example["leader_rotation"] + ":throughput"]
            example_data[int(example["index"])][example["leader_rotation"] + ":latency"] = example[example["leader_rotation"] + ":latency"]
    example_data = example_data[:max_index + 1]
    pd.DataFrame(example_data).to_excel(f"{test_dir}.xlsx", index=False)
    pd.DataFrame(example_data).to_csv(f"{test_dir}.csv", index=False)


def draw_base():
    draw_base_throughput()
    draw_base_latency()


def draw_base_throughput():
    df = pd.read_csv("test_base_merge.csv")
    fig, axs = plt.subplots(nrows=3, ncols=3, figsize=(10, 5))
    cnt = 0
    grouped = df.groupby("payload_size")
    for group in grouped:
        for batch_group in group[1].groupby("batch_size"):
            for consensus in leader_rotation_list + consensus_list:
                x = batch_group[1]["rate_limit"]
                y = batch_group[1][consensus + ":throughput"]
                axs[int(cnt/3)][cnt % 3].plot(x, y, label=consensus)
                axs[int(cnt/3)][cnt % 3].set_title(f"batch_size={batch_group[0]}")
            cnt += 1
    plt.legend()
    plt.savefig("test_base_throughput.png")
    plt.show()


def draw_base_latency():
    df = pd.read_csv("test_base_merge.csv")
    fig, axs = plt.subplots(nrows=3, ncols=3, figsize=(10, 5))
    cnt = 0
    grouped = df.groupby("payload_size")
    for group in grouped:
        for batch_group in group[1].groupby("batch_size"):
            for consensus in leader_rotation_list + consensus_list:
                x = batch_group[1]["rate_limit"]
                y = batch_group[1][consensus + ":latency"]
                axs[int(cnt/3)][cnt % 3].plot(x, y)
                axs[int(cnt/3)][cnt % 3].set_title(f"batch_size={batch_group[0]}")
            cnt += 1
    plt.legend()
    plt.savefig("test_base_latency.png")
    plt.show()


def reputation():
    leader_rotations = ["reputation", "round-robin"]
    batch_size = 1
    max_concurrent = 1
    rate_limit = 100
    replicas = 4
    byzantines = ["", '"--byzantine silence:1"']
    for leader_rotation in leader_rotations:
        for byzantine in byzantines:
            target_file = f"test_reputation/test_reputation_{leader_rotation}_{replicas}_byzantine{not byzantine==''}"
            if not os.path.exists(target_file):
                cmd = f"./test_base.sh {replicas} {leader_rotation} {batch_size} {max_concurrent} {int(rate_limit/client_num)} {target_file} {byzantine}"
                os.system(cmd)


def resolve_reputation_data(test_dir):
    for example in os.listdir(test_dir):
        replica_reputations = {}
        if test_dir not in example:
            continue
        args = example.split(test_dir + "_")[1].split("_")
        leader_rotation = args[0]
        replicas = args[1]
        use_byzantine = args[2].split("byzantine")[1]
        print(f"leader_rotation: {leader_rotation}, replicas: {replicas}, use_byzantine: {use_byzantine}")
        if leader_rotation == "reputation":
            with open(os.path.join(test_dir, example, "test.log"), 'r') as fp:
                for line in fp.readlines():
                    if "reputation" not in line:
                        continue
                    pattern = r'id=(\d+),\s+view=(\d+),\s+SR=([\d.]+),\s+LR=([\d.]+),\s+B=([\d.]+),\s+A=([\d.]+),\s+S=([\d.]+),\s+P=([\d.]+)'
                    match = re.search(pattern, line)
                    if match:
                        id_val = match.group(1)
                        view_val = match.group(2)
                        SR_val = match.group(3)
                        LR_val = match.group(4)
                        B_val = match.group(5)
                        A_val = match.group(6)
                        S_val = match.group(7)
                        P_val = match.group(8)
                        if id_val not in replica_reputations:
                            replica_reputations[id_val] = []
                        replica_reputations[id_val].append({
                            "view": view_val,
                            "SR": SR_val,
                            "LR": LR_val,
                            "B": B_val,
                            "A": A_val,
                            "S": S_val,
                            "P": P_val
                        })
                    else:
                        continue
            for replica in replica_reputations:
                pd.DataFrame(replica_reputations[replica]).to_csv(f"{test_dir}/csv/byzantine:{use_byzantine}/{replica}.csv", index=False)


def draw_reputation(test_dir):
    fig, axs = plt.subplots(nrows=2, ncols=2, figsize=(10, 5))
    for csv in os.listdir(test_dir):
        df = pd.read_csv(os.path.join(test_dir, csv))
        id = int(csv.split(".")[0])
        x = df["view"]
        LR = df["LR"]
        SR = df["SR"]
        B = df["B"]
        axs[int((id-1)/2)][(id-1) % 2].plot(x, LR, label="LR")
        axs[int((id-1)/2)][(id-1) % 2].plot(x, SR, label="SR")
        axs[int((id-1)/2)][(id-1) % 2].plot(x, B, label="B")
    plt.legend()
    plt.savefig(os.path.join(test_dir, "reputation.png"))
    plt.show()


def byzantine():
    leader_rotations = ["reputation", "round-robin"]
    batch_size = 100
    max_concurrent = 100
    rate_limit = 20000
    replicas = 10
    payload_size = 0
    byzantines = ["", '"--byzantine silence:1"', '"--byzantine silence:2"', '"--byzantine silence:3"']
    new_epoch_rate = 0
    new_epoch_duration = "2s"
    for index, byzantine in enumerate(byzantines):
        for leader_rotation in leader_rotations:
            target_file = f"test_byzantine/test_byzantine_{leader_rotation}_{replicas}_byzantine_{index}"
            print(f"start test: leader_rotation: {leader_rotation}, replicas: {replicas}, byzantine: {byzantine}")
            if not os.path.exists(target_file):
                cmd = f"./test_base.sh {replicas} {leader_rotation} {batch_size} {max_concurrent} {int(rate_limit/client_num)} {target_file} {payload_size} {new_epoch_rate} {new_epoch_duration} {byzantine} > /dev/null 2>&1"
                os.system(cmd)


def draw_byzantine(test_dir):
    fig, axs = plt.subplots(nrows=1, ncols=2, figsize=(6, 4))
    df = pd.read_csv(os.path.join(test_dir, "data.csv"))
    x = df["byzantine"]
    reputation_throughput = df["throughput:reputation"]
    round_robin_throughput = df["throughput:round-robin"]
    latency_reputation = df["latency:reputation"]
    latency_round_robin = df["latency:round-robin"]
    axs[0].plot(x, reputation_throughput, label="reputation")
    axs[0].plot(x, round_robin_throughput, label="round-robin")
    axs[1].plot(x, latency_reputation, label="reputation")
    axs[1].plot(x, latency_round_robin, label="round-robin")
    plt.legend()
    plt.savefig(os.path.join(test_dir, "byzantine.png"))
    plt.show()


def scalability():
    leader_rotations = ["reputation", "round-robin"]
    batch_size = 400
    max_concurrent = 400
    rate_limit = 5000
    replicas_list = [4, 8, 16, 32, 64, 128]
    for replicas in replicas_list:
        for leader_rotation in leader_rotations:
            target_file = f"test_scalability/test_scalability_{leader_rotation}_{replicas}"
            print(f"start test: leader_rotation: {leader_rotation}, replicas: {replicas}")
            if not os.path.exists(target_file):
                cmd = f"./test_base.sh {replicas} {leader_rotation} {batch_size} {max_concurrent} {int(rate_limit/client_num)} {target_file} > /dev/null 2>&1"
                os.system(cmd)


def new_epoch():
    leader_rotations = ["reputation", "round-robin"]
    batch_size = 100
    max_concurrent = 100
    rate_limit = 20000
    replicas = 8
    payload_size = 128
    new_epoch_rate = 0.5
    new_epoch_duration = "6s"
    for leader_rotation in leader_rotations:
        target_file = f"test_new_epoch/test_new_epoch_{leader_rotation}_{replicas}"
        print(f"start test: leader_rotation: {leader_rotation}, replicas: {replicas}")
        if not os.path.exists(target_file):
            cmd = f"./test_base.sh {replicas} {leader_rotation} {batch_size} {max_concurrent} {int(rate_limit/client_num)} {target_file} {payload_size} {new_epoch_rate} {new_epoch_duration} > /dev/null 2>&1"
            os.system(cmd)


def resolve_new_epoch(test_dir):
    os.system(f"cd {test_dir}/test_new_epoch_reputation_8; cat test.log | grep hs4 > test-hs4.log")
    commit_reputation_log = os.path.join(test_dir, "test_new_epoch_reputation_8", "test-hs4.log")
    with open(commit_reputation_log, 'r') as fp:
        start_time = 0
        time2height = []
        txn = 0
        for line in fp.readlines():
            if "commit block" in line:
                iso_string = line.split('\t')[0]
                dt_object = datetime.strptime(iso_string[:-1].replace('T', ' '), '%Y-%m-%d %H:%M:%S.%f')
                # 转换为时间戳（秒为单位）
                timestamp = dt_object.timestamp()
                height = int(line.split("height=")[1].split(",")[0])
                txn += int(line.split("txn=")[1].split(",")[0])
                if height == 1:
                    start_time = timestamp
                t = timestamp - start_time
                time2height.append({"time": t, "height": height, "txn": txn})
    reputation_df = pd.DataFrame(time2height)
    os.system(f"cd {test_dir}/test_new_epoch_round-robin_8; cat test.log | grep hs4 > test-hs4.log")
    round_robin_log = os.path.join(test_dir, "test_new_epoch_round-robin_8", "test-hs4.log")
    with open(round_robin_log, 'r') as fp:
        start_time = 0
        time2height = []
        txn = 0
        for line in fp.readlines():
            if "commit block" in line:
                iso_string = line.split('\t')[0]
                dt_object = datetime.strptime(iso_string[:-1].replace('T', ' '), '%Y-%m-%d %H:%M:%S.%f')
                # 转换为时间戳（秒为单位）
                timestamp = dt_object.timestamp()
                height = int(line.split("height=")[1].split(",")[0])
                if height == 1:
                    start_time = timestamp
                t = timestamp - start_time
                txn += int(line.split("txn=")[1].split(",")[0])
                time2height.append({"time": t, "height": height, "txn": txn})
    round_robin_df = pd.DataFrame(time2height)
    plt.plot(reputation_df["time"], reputation_df["txn"], label="reputation")
    plt.plot(round_robin_df["time"], round_robin_df["txn"], label="round-robin")
    plt.legend()
    plt.savefig(os.path.join(test_dir, "new_epoch.png"))
    plt.show()


def resolve_new_epoch_reputation(test_dir):
    replica_reputations = {}
    with open(os.path.join(test_dir, "test_new_epoch_reputation_8", "test.log")) as fp:
        for line in fp.readlines():
            if "reputation" not in line:
                continue
            pattern = r'id=(\d+),\s+view=(\d+),\s+SR=([\d.]+),\s+LR=([\d.]+),\s+B=([\d.]+),\s+A=([\d.]+),\s+S=([\d.]+),\s+P=([\d.]+)'
            match = re.search(pattern, line)
            if match:
                id_val = match.group(1)
                view_val = match.group(2)
                SR_val = match.group(3)
                LR_val = match.group(4)
                B_val = match.group(5)
                A_val = match.group(6)
                S_val = match.group(7)
                P_val = match.group(8)
                if id_val not in replica_reputations:
                    replica_reputations[id_val] = []
                replica_reputations[id_val].append({
                    "view": view_val,
                    "SR": SR_val,
                    "LR": LR_val,
                    "B": B_val,
                    "A": A_val,
                    "S": S_val,
                    "P": P_val
                })
            else:
                continue

    for replica in replica_reputations:
        df = pd.DataFrame(replica_reputations[replica])
        id = int(replica)
        x = df["view"]
        LR = df["LR"]
        SR = df["SR"]
        B = df["B"]
        plt.plot(x, LR, label="replica" + str(id))
    plt.legend()
    plt.savefig(os.path.join(test_dir, "reputation.png"))
    plt.show()



def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--type", help="实验类型", default=False)
    args = parser.parse_args()
    if args.type == "base":
        base()
        example_need_retry = resolve_data("test_base")
        while example_need_retry is not None:
            # 删除example_need_retry
            print(f"retry example: {example_need_retry}")
            os.system(f"rm -rf test_base/{example_need_retry}")
            base()
            example_need_retry = resolve_data("test_base")
        draw_base()
    if args.type == "reputation":
        reputation()
        resolve_reputation_data("test_reputation")
        draw_reputation("test_reputation/csv/byzantine:False")
        draw_reputation("test_reputation/csv/byzantine:True")
    if args.type == "byzantine":
        byzantine()
        draw_byzantine("test_byzantine")
    if args.type == "scalability":
        scalability()
    if args.type == "new_epoch":
        new_epoch()
        resolve_new_epoch("test_new_epoch")
        # resolve_new_epoch_reputation("test_new_epoch")


if __name__ == "__main__":
    main()
