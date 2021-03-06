package healthcheck

import (
	"context"
	"fmt"

	pb "github.com/runconduit/conduit/controller/gen/common"
	"google.golang.org/grpc"
)

type grpcStatusChecker interface {
	SelfCheck(ctx context.Context, in *pb.SelfCheckRequest, opts ...grpc.CallOption) (*pb.SelfCheckResponse, error)
}

type statusCheckerProxy struct {
	delegate grpcStatusChecker
	prefix   string
}

func (proxy *statusCheckerProxy) SelfCheck() ([]CheckResult, error) {
	translatedResults := make([]CheckResult, 0)

	canConnectViaGrpcCheck := CheckResult{
		SubsystemName:    proxy.prefix,
		CheckDescription: "can retrieve status via gRPC",
		Status:           CheckError,
	}
	selfCheckResponse, err := proxy.delegate.SelfCheck(context.Background(), &pb.SelfCheckRequest{})
	if err != nil {
		canConnectViaGrpcCheck.Status = CheckError
		canConnectViaGrpcCheck.FriendlyMessageToUser = err.Error()
	} else {
		canConnectViaGrpcCheck.Status = CheckOk

		for _, check := range selfCheckResponse.Results {
			fullSubsystemName := fmt.Sprintf("%s[%s]", proxy.prefix, check.SubsystemName)

			var status CheckStatus

			switch CheckStatus(check.Status) {
			case CheckOk, CheckFailed, CheckError:
				status = CheckStatus(check.Status)
			default:
				status = CheckError
			}

			translatedResults = append(translatedResults, CheckResult{
				SubsystemName:         fullSubsystemName,
				CheckDescription:      check.CheckDescription,
				Status:                status,
				FriendlyMessageToUser: check.FriendlyMessageToUser,
			})
		}
	}

	subsystemResults := []CheckResult{
		canConnectViaGrpcCheck,
	}
	subsystemResults = append(subsystemResults, translatedResults...)
	return subsystemResults, nil
}

func NewGrpcStatusChecker(name string, grpClient grpcStatusChecker) StatusChecker {
	return &statusCheckerProxy{
		prefix:   name,
		delegate: grpClient,
	}
}
