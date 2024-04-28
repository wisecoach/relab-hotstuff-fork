#!/home/wisecoach/anaconda3/bin/python
import argparse
import json
import os

import argparse

block_duration = 0
block_sum = 0
throughput_duration = 0
throughput_sum = 0
latency_duration = 0
latency_sum = 0
latency_variance_sum = 0


def handle_data(dir):
    for worker in os.listdir(dir):
        json_file = os.path.join(dir, worker, "measurements.json")
        if os.path.isdir(os.path.join(dir, worker)):
            with open(json_file) as fp:
                events = json.load(fp)
                process_events(events)


def process_events(events):
    global throughput_duration, throughput_sum, block_duration, block_sum, latency_duration, latency_sum
    for event in events:
        if event['@type'] == "type.googleapis.com/types.ThroughputMeasurement":
            throughput_duration += float(event['Duration'][:-1])
            throughput_sum += int(event['Commands'])
            block_duration += float(event['Duration'][:-1])
            block_sum += int(event['Commits'])
        if event['@type'] == "type.googleapis.com/types.LatencyMeasurement":
            if int(event['Count']) == 0:
                continue
            latency_duration += int(event['Count'])
            latency_sum += float(event['Latency']) * int(event['Count'])




def main():
    # 创建一个解析器对象
    parser = argparse.ArgumentParser()

    # 添加参数
    parser.add_argument("dir", help="要解析的实验数据文件夹")

    # 解析参数
    args = parser.parse_args()


    handle_data(args.dir)

    print("block throughput: ", block_sum / block_duration)
    print("throughput: ", throughput_sum / throughput_duration)
    print("latency: ", latency_sum / latency_duration)


if __name__ == "__main__":
    main()
