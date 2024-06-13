import pandas as pd
import matplotlib.pyplot as plt

data = pd.read_csv('benchmark_results.csv')

data['Size'] = data['Size'].astype(int)

plt.figure(figsize=(12, 8))
for msgs in data['MessagesToSend'].unique():
    subset = data[data['MessagesToSend'] == msgs]
    plt.plot(subset['Size'], subset['DroppedMessages'], label=f'MessagesToSend {msgs}')

plt.xlabel('Producer Channel Size')
plt.ylabel('Average Dropped Messages')
plt.title('Benchmark of InfiniteChannel: Dropped Messages vs. Channel Size')
plt.legend()
plt.grid(True)
plt.savefig('benchmark_results_1.png')
plt.show()
