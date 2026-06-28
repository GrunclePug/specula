package ping

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Ping performs multiple ping checks on a target to calculate packet loss and average latency.
// If host contains a colon (e.g. host:port), it performs multiple TCP connection checks.
// Otherwise, it runs the system ICMP "ping" command sending multiple packets.
func Ping(host string, timeout time.Duration, count int) (float64, time.Duration, error) {
	if strings.Contains(host, ":") {
		// TCP Ping with multiple attempts to measure loss
		successCount := 0
		var totalDuration time.Duration
		attemptTimeout := timeout / time.Duration(count)

		for i := 0; i < count; i++ {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", host, attemptTimeout)
			duration := time.Since(start)
			if err == nil {
				successCount++
				totalDuration += duration
				_ = conn.Close()
			}
			// Small cooldown between attempts
			time.Sleep(20 * time.Millisecond)
		}

		loss := float64(count-successCount) / float64(count) * 100
		var avgRTT time.Duration
		if successCount > 0 {
			avgRTT = totalDuration / time.Duration(successCount)
		}

		if successCount == 0 {
			return 100, 0, fmt.Errorf("all TCP connection attempts failed")
		}
		return loss, avgRTT, nil
	}

	// ICMP Ping via system ping command
	timeoutSecs := int(timeout.Seconds())
	if timeoutSecs < 1 {
		timeoutSecs = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// -c <count>: send <count> packets
	// -i 0.2: send interval of 0.2 seconds (standard users can go down to 0.2)
	// -W <seconds>: wait for response timeout (in seconds)
	cmd := exec.CommandContext(
		ctx,
		"ping",
		"-c", strconv.Itoa(count),
		"-i", "0.2",
		"-W", strconv.Itoa(timeoutSecs),
		host,
	)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)

	loss := 100.0
	var avgRTT time.Duration
	lossParsed := false

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Parse packet loss: e.g., "5 packets transmitted, 5 received, 0% packet loss"
		if strings.Contains(line, "packet loss") {
			idx := strings.Index(line, "% packet loss")
			if idx != -1 {
				startIdx := idx
				for startIdx > 0 && line[startIdx-1] != ' ' && line[startIdx-1] != ',' {
					startIdx--
				}
				lossStr := line[startIdx:idx]
				parsedLoss, parseErr := strconv.ParseFloat(strings.TrimSpace(lossStr), 64)
				if parseErr == nil {
					loss = parsedLoss
					lossParsed = true
				}
			}
		}

		// Parse RTT statistics: e.g., "rtt min/avg/max/mdev = 10.123/12.345/14.567/1.234 ms"
		if strings.Contains(line, "min/avg/max") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				vals := strings.Split(strings.TrimSpace(parts[1]), "/")
				if len(vals) >= 2 {
					avgRTTStr := strings.TrimSpace(vals[1])
					// Strip units if present (e.g. " ms")
					avgRTTStr = strings.Fields(avgRTTStr)[0]
					avgRTTVal, parseErr := strconv.ParseFloat(avgRTTStr, 64)
					if parseErr == nil {
						avgRTT = time.Duration(avgRTTVal * float64(time.Millisecond))
					}
				}
			}
		}
	}

	if err != nil && !lossParsed {
		return 100, 0, err
	}

	if loss == 100 {
		return 100, 0, fmt.Errorf("100%% packet loss")
	}

	return loss, avgRTT, nil
}

