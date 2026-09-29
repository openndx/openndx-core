package models

// Member represents the normalized entity table
type Member struct {
	MemberID    string `gorm:"primarykey;column:member_id" json:"memberId"`
	Name        string `gorm:"column:name;not null" json:"name"`
	Email       string `gorm:"column:email;not null;unique" json:"email"`
	PhoneNumber string `gorm:"column:phone_number;not null" json:"phoneNumber"`
	IdpUserID   string `gorm:"column:idp_user_id;not null;unique" json:"idpUserId"`
	BaseModel
}

// TableName sets the table name for GORM
func (Member) TableName() string {
	return "members"
}
