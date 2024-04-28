#!/home/wisecoach/anaconda3/bin/python
import argparse
import re
import pandas as pd


def handle_data(log, target):
    with open(log, 'r') as fp:
        reputation_dict = {"view": []}
        df = pd.DataFrame(reputation_dict)
        for line in fp.readlines():
            if not (line.__contains__("reputation:") and line.__contains__("INFO")):
                continue
            pattern = r"hs\d+|(\d+)"
            matches = re.findall(pattern, line)
            if len(matches) < 5:
                continue
            values = matches[-5:]
            view = values[-1]
            replica = "hs" + values[0]
            df.loc[view, "view"] = view
            if replica + ":SR" not in df.columns:
                df[replica + ":SR"] = [0]*df.shape[0]
            df.loc[view, replica + ":SR"] = values[1]
            if replica + ":LR" not in df.columns:
                df[replica + ":LR"] = [0]*df.shape[0]
            df.loc[view, replica + ":LR"] = values[2]
            if replica + ":RR" not in df.columns:
                df[replica + ":RR"] = [0]*df.shape[0]
            df.loc[view, replica + ":RR", ] = values[3]
        df.to_csv(target, index=False)


def main():
    # 创建一个解析器对象
    parser = argparse.ArgumentParser()
    parser.add_argument("--log", help="日志文件", default="test.log")
    parser.add_argument("--target", help="目标文件夹", default="reputation.csv")
    args = parser.parse_args()
    handle_data(args.log, args.target)


if __name__ == "__main__":
    main()
