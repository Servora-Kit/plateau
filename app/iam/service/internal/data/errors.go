package data

import (
	"errors"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
)

func translateEntError(err error) error {
	if err == nil {
		return nil
	}
	if entmodel.IsNotFound(err) {
		return biz.ErrNotFound
	}
	if entmodel.IsConstraintError(err) {
		return errors.Join(biz.ErrAlreadyExists, err)
	}
	return err
}
