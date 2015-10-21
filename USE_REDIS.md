# Redis as a task set

* one set of files to copy ('todo')
* every processor has its own active set
* every processor has a 'freshness' key

## task creators

One or more processes add tasks to the 'todo' set:

```bash
redis-cli sadd todo $task
```

## task processors

On startup, a processor generates some uuid and initiates its freshness:

```bash
redis-cli set processor_$id $(date +%s)
```

Every n seconds, every task processor refreshes its freshness key

```bash
redis-cli set processor_$id $(date +%s)
```

On clean shutdown, a task processor cleans up after itself:

One or more processes move random tasks from the 'todo' set to their own active set

```bash
task="$(redis-cli --raw spop todo)"
redis-cli sadd active_$id $task
```

```bash
redis-cli del processor_$id
redis-cli del active_$id
```

## nanny process

If a task processor's freshness key turns rotten, all items on its active set are moved back to the 'todo' set and its items are removed

```bash
TIMEDIFF=10 # max 10 sec freshness
MINTIME=$(date +"%s" -d "$TIMEDIFF seconds ago"
for processor_state in $(redis-cli --raw keys 'processor_*')
do
	processor_id=${processor_state##processor_}
	processor_freshness=$(redis-cli --raw get $processor_state)
	if [[ "$processor_freshness" -lt "$MINTIME" ]]
	then
		processor_queue="active_$processor_id"
		redis-cli sunionstore todo $processor_queue todo
		redis-cli del $processor_queue
		redis-cli del $processor_state
		MINTIME=$(date +"%s" -d "$TIMEDIFF seconds ago"
	fi
done

# tasks

Tasks are added to the todo set, and to a second hash containing the metadata about the task:

```bash
redis-cli sadd todo $task
redis-cli hset task_info $task "src:/srcpath/a,dst:/dstpath/a,..." # or some other encoding
```
